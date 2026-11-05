import {
  createBrowserRouter, Navigate, Outlet, useNavigate, RouterProvider, useRouteError,
} from "react-router";
import { useAuth } from "../features/auth/store";
import { Button } from "@/components/ui/button";
import { ShellLayout } from "./layout";
import { Login } from "../features/auth/Login";
import { Setup } from "../features/auth/Setup";
import { Overview } from "../features/overview/Overview";
import { useI18n } from "../shared/i18n";
import { Proxy } from "../features/proxy/Proxy";
import { Deployments } from "../features/deployments/Deployments";
import { CI } from "../features/ci/CI";
import { Logs } from "../features/logs/Logs";
import { AnalyticsPage } from "../features/overview/AnalyticsPage";
import { Observability } from "../features/overview/Observability";
import { Alerts } from "../features/alerts/Alerts";
import { EnvAll } from "../features/env/EnvAll";
import { AppTabs } from "../features/apps/AppTabs";
import { AppOverview } from "../features/apps/AppOverview";
import { AppUsage } from "../features/apps/AppUsage";
import { AppEnv } from "../features/apps/AppEnv";
import { AppSettings } from "../features/apps/AppSettings";
import { Routes } from "../features/routes/Routes";
import { Settings } from "../features/settings/Settings";
import { Users } from "../features/users/Users";

// Puerta auth: setup primero, luego login. Solo guarda, no pinta.
// Espera boot completo (ready): sin esto, recargar un deep link
// rebotaba /ruta → /login → /overview (me() asentaba tarde).
function RequireAuth() {
  const { ready, authed, setupRequired } = useAuth();
  if (!ready) return null;
  if (setupRequired) return <Navigate to="/setup" replace />;
  if (!authed) return <Navigate to="/login" replace />;
  return <Outlet />;
}

function LoginRoute() {
  const { ready, authed, setupRequired, setWho } = useAuth();
  const navigate = useNavigate();
  if (!ready) return null;
  if (setupRequired) return <Navigate to="/setup" replace />;
  if (authed) return <Navigate to="/overview" replace />;
  return (
    <Login
      onOk={(m) => {
        setWho(m);
        navigate("/overview", { replace: true });
      }}
    />
  );
}

function SetupRoute() {
  const { ready, setupRequired, setWho } = useAuth();
  const navigate = useNavigate();
  if (!ready) return null;
  if (setupRequired === false) return <Navigate to="/login" replace />;
  return (
    <Setup
      onOk={(m) => {
        setWho(m);
        navigate("/overview", { replace: true });
      }}
    />
  );
}

// Polling: useFleet (montado en layout + página) sondea cada 5s;
// respuestas cacheadas 4s = una request aunque haya N montados.
function UsersRoute() {
  const { isAdmin } = useAuth();
  if (!isAdmin) return <Navigate to="/overview" replace />;
  return <Users />;
}

function AppDeploysTab() {
  return <Deployments />;
}

function AppLogsTab() {
  return <Logs />;
}

// Fallback de render: un error de vista no debe blanquear la app entera.
function RouteError() {
  const { t } = useI18n();
  const err = useRouteError();
  const msg = err instanceof Error ? err.message : String(err ?? t("shell.unknownError"));
  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <div className="w-full max-w-lg border border-bad/40 bg-card/60 p-4">
        <div className="flex items-center gap-2">
          <span aria-hidden className="h-1.5 w-1.5 bg-bad" />
          <p className="micro text-bad">{t("shell.interfaceError")}</p>
        </div>
        <p className="mt-2 font-mono text-[11px] leading-relaxed break-words text-muted-foreground">{msg}</p>
        <div className="mt-4 flex flex-wrap gap-2">
          <Button size="sm" onClick={() => window.location.reload()}>
            {t("shell.reload")}
          </Button>
          <Button size="sm" variant="outline" onClick={() => { window.location.href = "/overview"; }}>
            {t("shell.goOverview")}
          </Button>
        </div>
      </div>
    </div>
  );
}

export const router = createBrowserRouter(
  [
    { path: "/login", element: <LoginRoute />, errorElement: <RouteError /> },
    { path: "/setup", element: <SetupRoute />, errorElement: <RouteError /> },
    {
      path: "/",
      element: <RequireAuth />,
      errorElement: <RouteError />,
      children: [
        {
          element: <ShellLayout />,
          children: [
            { index: true, element: <Navigate to="/overview" replace /> },
            { path: "overview", element: <Overview /> },
            { path: "proxy", element: <Proxy /> },
            { path: "deployments", element: <Deployments /> },
            { path: "ci", element: <CI /> },
            { path: "logs", element: <Logs /> },
            { path: "analytics", element: <AnalyticsPage /> },
            { path: "observability", element: <Observability /> },
            { path: "alerts", element: <Alerts /> },
            { path: "environment", element: <EnvAll /> },
            {
              path: "app/:name",
              element: <AppTabs />,
              children: [
                { index: true, element: <Navigate to="overview" replace /> },
                { path: "overview", element: <AppOverview /> },
                { path: "usage", element: <AppUsage /> },
                { path: "deploys", element: <AppDeploysTab /> },
                { path: "logs", element: <AppLogsTab /> },
                { path: "analytics", element: <AnalyticsPage /> },
                { path: "env", element: <AppEnv /> },
                { path: "settings", element: <AppSettings /> },
              ],
            },
            { path: "routes", element: <Routes /> },
            { path: "settings", element: <Settings /> },
            { path: "users", element: <UsersRoute /> },
            { path: "*", element: <Navigate to="/overview" replace /> },
          ],
        },
      ],
    },
  ],
);

export function AppRouter() {
  return <RouterProvider router={router} />;
}
