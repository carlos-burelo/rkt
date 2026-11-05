import {
  LayoutGrid, Rocket, Hammer, Terminal, BarChart3, Activity,
  Bell, Globe, FlaskConical, Settings, Users, Server,
} from "lucide-react";

// Navegación del shell. `labelKey` se traduce en el componente; `admin`
// oculta el item a no-admin.
export const GROUPS = [
  {
    labelKey: "nav.group.operate",
    items: [
      { to: "/overview", labelKey: "nav.overview", icon: LayoutGrid },
      { to: "/deployments", labelKey: "nav.deployments", icon: Rocket },
      { to: "/ci", labelKey: "nav.ci", icon: Hammer },
      { to: "/logs", labelKey: "nav.logs", icon: Terminal },
    ],
  },
  {
    labelKey: "nav.group.serve",
    items: [
      { to: "/routes", labelKey: "nav.routes", icon: Globe },
      { to: "/environment", labelKey: "nav.environment", icon: FlaskConical },
    ],
  },
  {
    labelKey: "nav.group.observe",
    items: [
      { to: "/server", labelKey: "nav.server", icon: Server },
      { to: "/analytics", labelKey: "nav.analytics", icon: BarChart3 },
      { to: "/observability", labelKey: "nav.observability", icon: Activity },
      { to: "/alerts", labelKey: "nav.alerts", icon: Bell },
    ],
  },
  {
    labelKey: "nav.group.system",
    items: [
      { to: "/settings", labelKey: "nav.settings", icon: Settings },
      { to: "/users", labelKey: "nav.users", icon: Users, admin: true },
    ],
  },
] as const;

// clave del título por primer segmento de ruta.
export const PAGE_LABEL: Record<string, string> = {
  overview: "nav.overview",
  deployments: "nav.deployments",
  ci: "nav.ci",
  logs: "nav.logs",
  routes: "nav.routes",
  server: "nav.server",
  environment: "nav.environment",
  analytics: "nav.analytics",
  observability: "nav.observability",
  alerts: "nav.alerts",
  settings: "nav.settings",
  users: "nav.users",
  app: "nav.app",
};
