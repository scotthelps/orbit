import Vue from "vue";
import Router from "vue-router";
import Meta from "vue-meta";

Vue.use(Router);
Vue.use(Meta, { keyName: "meta" });

import SetupView from "@/views/Setup";
import LoginView from "@/views/Login";

// All of the primary views.
import MainView from "@/views/Main";

import NodesView from "@/views/Nodes";

import NamespaceView from "@/views/Namespace";
import NamespacesView from "@/views/Namespaces";
import NewNamespaceView from "@/views/NewNamespace";

import UsersView from "@/views/Users";
import SecurityView from "@/views/Security";

import OverviewView from "@/views/Overview";
import RepositoriesView from "@/views/Repositories";
import DeploymentsView from "@/views/Deployments";
import RoutersView from "@/views/Routers";
import CertificatesView from "@/views/Certificates";
import VolumesView from "@/views/Volumes";

import NotFoundView from "@/views/NotFound";

const routes = [
  { path: "/setup", component: SetupView },
  { path: "/login", component: LoginView },
  {
    path: "/",
    component: MainView,
    children: [
      /**
       * NODES.
       */
      {
        path: "/nodes",
        component: NodesView
      },

      /**
       * NAMESPACES.
       */
      {
        path: "/namespaces",
        component: NamespacesView
      },
      {
        path: "/namespaces/new",
        components: { default: NamespacesView, slider: NewNamespaceView }
      },
      {
        path: "/namespaces/:id",
        components: { default: NamespacesView, slider: NamespaceView }
      },

      { path: "/users", component: UsersView },
      { path: "/security", component: SecurityView },

      { path: "", component: OverviewView },
      { path: "/repositories", component: RepositoriesView },
      { path: "/deployments", component: DeploymentsView },
      { path: "/routers", component: RoutersView },
      { path: "/certificates", component: CertificatesView },
      { path: "/volumes", component: VolumesView },

      { path: "*", component: NotFoundView }
    ]
  }
];

const mode = "history";
const router = new Router({ routes, mode });
export default router;
