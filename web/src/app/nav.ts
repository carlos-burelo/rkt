import {
  LayoutGrid, Network, Rocket, Hammer, Terminal, BarChart3, Activity,
  Bell, Globe, FlaskConical, Settings, Users,
} from "lucide-react";

// Navegación del shell en un solo hogar. `admin` oculta el item a no-admin.
export const GROUPS = [
  {
    label: "Operar",
    items: [
      { to: "/overview", label: "Resumen", icon: LayoutGrid },
      { to: "/deployments", label: "Despliegues", icon: Rocket },
      { to: "/ci", label: "CI", icon: Hammer },
      { to: "/logs", label: "Registros", icon: Terminal },
    ],
  },
  {
    label: "Servir",
    items: [
      { to: "/routes", label: "Dominios", icon: Globe },
      { to: "/proxy", label: "Proxy", icon: Network },
      { to: "/environment", label: "Entorno", icon: FlaskConical },
    ],
  },
  {
    label: "Observar",
    items: [
      { to: "/analytics", label: "Analítica", icon: BarChart3 },
      { to: "/observability", label: "Observabilidad", icon: Activity },
      { to: "/alerts", label: "Alertas", icon: Bell },
    ],
  },
  {
    label: "Sistema",
    items: [
      { to: "/settings", label: "Ajustes", icon: Settings },
      { to: "/users", label: "Usuarios", icon: Users, admin: true },
    ],
  },
] as const;

export const PAGE_LABEL: Record<string, string> = {
  overview: "Resumen",
  deployments: "Despliegues",
  ci: "CI",
  logs: "Registros",
  routes: "Dominios",
  proxy: "Proxy",
  environment: "Entorno",
  analytics: "Analítica",
  observability: "Observabilidad",
  alerts: "Alertas",
  settings: "Ajustes",
  users: "Usuarios",
  app: "App",
};
