package engine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"orbit.sh/engine/docker"
)

// Deployment is a store instance of a deployment created from an image or
// repository. It could also be referred to as an "App" (and in some cases
// throughout Orbit, actually is).
type Deployment struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// The location that the deployment is created from.
	Repository string `json:"repository"`
	Branch     string `json:"branch"` // The branch to use, if not set, will default to "master"
	Path       string `json:"path"`   // A subdirectory or root of the repo

	// The logs from the build processes. This is a map that contains a string
	// (the key) which is used to store the git commit hash of the repository that
	// this particular deployment path was taken from. The value is a string list
	// of the individual lines outputted from the build process. These all need to
	// be kept in raft consensus so that they can be referenced later on.
	BuildLogs map[string][]string `json:"build_logs"`

	NamespaceID string `json:"namespace_id"`
}

// Deployments is a slice of the deployments in the store.
type Deployments []Deployment

// AppendBuildLog appends a log item entry to a given build process with a given
// ID. This takes place in a deployment, and is a short-hand for the longer
// process of performing a store apply.
func (s *Store) AppendBuildLog(deploymentID, key string, lines ...string) error {
	// Don't perform an operation if this command contained no additional lines.
	if len(lines) == 0 {
		return nil
	}

	// Create the log map so that the apply command knows what data we need to
	// provide it. Set the lines that we provide to it as the log items to use.
	logs := map[string][]string{}
	logs[key] = lines

	// Construct the apply command.
	cmd := command{
		Op: opAppendBuildLog,
		Deployment: Deployment{
			ID:        deploymentID,
			BuildLogs: logs,
		},
	}

	// Apply it to the store.
	if err := cmd.Apply(s); err != nil {
		return err
	}

	return nil
}

// ClearBuildLog will clear the build log for a map with a given key. This will
// essentially remove the key in the map.
func (s *Store) ClearBuildLog(deploymentID, key string) error {
	// Create the empty log map to correctly inform the process to take place.
	logs := map[string][]string{}
	logs[key] = []string{}

	// Construct the command.
	cmd := command{
		Op: opClearBuildLog,
		Deployment: Deployment{
			ID:        deploymentID,
			BuildLogs: logs,
		},
	}

	// Apply the command.
	if err := cmd.Apply(s); err != nil {
		return err
	}

	return nil
}

// BuildDeployment will take in the given deployment object and then run through
// and actually perform the operations to build that deployment.
func (e *Engine) BuildDeployment(d Deployment) (string, error) {
	// Checkout the repo to a temporary directory, navigate to the specified path,
	// and if there is a Dockerfile, use that for building, and if there isn't,
	// create a default one that uses the herokuish image.

	// Find the repo.
	var repo *Repository
	for _, r := range e.Store.state.Repositories {
		if r.ID == d.Repository {
			repo = &r
			break
		}
	}
	if repo == nil {
		return "", fmt.Errorf("that repository does not exist")
	}

	// Derive the repo path.
	volume := e.Store.OrbitSystemVolume()
	if volume == nil {
		return "", fmt.Errorf("could not find the orbit system volume")
	}
	path := filepath.Join(volume.Paths().Data, "repositories", repo.ID)

	// Check it out to a temporary directory.
	tmp, err := ioutil.TempDir("", "")
	if err != nil {
		return "", fmt.Errorf("could not create temporary directory: %s", err)
	}
	cmd := exec.Command("git", "clone", path, tmp)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not run git clone command: %s", err)
	}

	// Ensure that we're in the correct branch (if it's set).
	if d.Branch != "" {
		cmd := exec.Command("git", "-C", tmp, "checkout", d.Branch)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", err
		}
	}

	// Retrieve the commit hash for this branch.
	cmd = exec.Command("git", "-C", tmp, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	hash := strings.TrimSpace(string(output))

	// Check for a Dockerfile and create one if there isn't one.
	src := filepath.Join(tmp, d.Path) // The actual directory to check
	if err := docker.EnsureDockerfile(src); err != nil {
		return "", fmt.Errorf("could not ensure dockerfile: %s", err)
	}

	// Generate the map key for the build log.
	now := fmt.Sprintf("%d", time.Now().UnixNano())
	key := filepath.Join(hash, now, d.Path)

	// flushBuffer takes in the buffer that is provided in the enclosing function
	// and updates the store deployment build log with the data in the buffer. If
	// there is no data in the buffer then this will not execute anything.
	var lineBuf []string
	flushBuffer := func() error {
		if err := e.Store.AppendBuildLog(d.ID, key, lineBuf...); err != nil {
			return err
		}
		lineBuf = []string{}
		return nil
	}

	// Begin the build process. All of the operations for this take place
	// asynchronously and so this is a non-blocking operation. Handle all of the
	// following output with the channels it creates.
	tag := fmt.Sprintf("127.0.0.1:6510/%s", d.ID)
	outputCh, errorCh := docker.Build(src, tag) // STARTS THE ASYNC OP
	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()

loop:
	for {
		select {
		// For each stdout line, append it to the buffer.
		case line, ok := <-outputCh:
			if !ok {
				break loop
			}
			lineBuf = append(lineBuf, line)
			fmt.Println(line)

		// If an error occurs at any point, return it and fail.
		case err := <-errorCh:
			return key, err

		// Every two seconds, actually save the buffer data to the store.
		case <-ticker.C:
			if err := flushBuffer(); err != nil {
				return key, err
			}
		}
	}

	// Perform a final flush of the buffer.
	if err := flushBuffer(); err != nil {
		return key, err
	}

	return key, nil
}

// GenerateID will create a unique identifier for the deployment.
func (d *Deployments) GenerateID() string {
search:
	for {
		b := make([]byte, 8)
		rand.Read(b)
		id := hex.EncodeToString(b)

		for _, deployment := range *d {
			if deployment.ID == id {
				continue search
			}
		}

		return id
	}
}
