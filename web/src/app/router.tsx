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
import { Deployments } from "../features/deployments/Deployments";
import { DeployPage } from "../features/deployments/DeployPage";
import { CI } from "../features/ci/CI";
import { Logs } from "../features/logs/Logs";
import { Cron } from "../features/cron/Cron";
import { AnalyticsPage } from "../features/overview/AnalyticsPage";
import { Observability } from "../features/overview/Observability";
import { Server } from "../features/server/Server";
import { Alerts } from "../features/alerts/Alerts";
import { EnvAll } from "../features/env/EnvAll";
import { AppTabs } from "../features/apps/AppTabs";
import { CreatePage } from "../features/apps/CreatePage";
import { AppOverview } from "../features/apps/AppOverview";
import { AppUsage } from "../features/apps/AppUsage";
import { AppEnv } from "../features/apps/AppEnv";
import { AppSettings } from "../features/apps/AppSettings";
import { Routes } from "../features/routes/Routes";
import { Databases } from "../features/databases/Databases";
import { Waf } from "../features/waf/Waf";
import { Help } from "../features/help/Help";
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
  const { ready, setupRequired, authed, isAdmin, setWho } = useAuth();
  const navigate = useNavigate();
  if (!ready) return null;
  // Sin usuarios: setup fresco. Con admin + sesión (reload a mitad del
  // wizard): se reanuda donde iba en vez de botar a /overview.
  if (setupRequired || (authed && isAdmin)) {
    return (
      <Setup
        onOk={(m) => {
          setWho(m);
          navigate("/overview", { replace: true });
        }}
      />
    );
  }
  if (!authed) return <Navigate to="/login" replace />;
  return <Navigate to="/overview" replace />;
}

// Polling: useFleet (montado en layout + página) sondea cada 5s;
// respuestas cacheadas 4s = una request aunque haya N montados.
function UsersRoute() {
  const { isAdmin } = useAuth();
  if (!isAdmin) return <Navigate to="/overview" replace />;
  return <Users />;
}

function WafRoute() {
  const { canWrite } = useAuth();
  if (!canWrite) return <Navigate to="/overview" replace />;
  return <Waf />;
}

function AppDeploysTab() {
  return <Deployments />;
}

function AppLogsTab() {
  return <Logs />;
}

// Ruta desconocida: se avisa en vez de rebotar en silencio al resumen.
// Un deep-link roto o escrito a mano merece su propio mensaje.
function NotFound() {
  const { t } = useI18n();
  const navigate = useNavigate();
  return (
    <div className="mx-auto mt-16 w-full max-w-md border border-border bg-card/60 p-6 text-center">
      <p className="micro text-muted-foreground">{t("shell.notFoundDesc")}</p>
      <h1 className="mt-2 text-[18px] font-medium">{t("shell.notFoundTitle")}</h1>
      <div className="mt-4">
        <Button size="sm" onClick={() => navigate("/overview", { replace: true })}>
          {t("shell.goOverview")}
        </Button>
      </div>
    </div>
  );
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
            { path: "proxy", element: <Navigate to="/routes?tab=diagnostico" replace /> },
            { path: "deployments", element: <Deployments /> },
            { path: "deployments/:id", element: <DeployPage /> },
            { path: "apps/new", element: <CreatePage /> },
            { path: "ci", element: <CI /> },
            { path: "logs", element: <Logs /> },
            { path: "cron", element: <Cron /> },
            { path: "analytics", element: <AnalyticsPage /> },
            { path: "observability", element: <Observability /> },
            { path: "server", element: <Server /> },
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
            { path: "databases", element: <Databases /> },
            { path: "waf", element: <WafRoute /> },
            { path: "settings", element: <Settings /> },
            { path: "help", element: <Help /> },
            { path: "users", element: <UsersRoute /> },
            { path: "*", element: <NotFound /> },
          ],
        },
      ],
    },
  ],
);

export function AppRouter() {
  return <RouterProvider router={router} />;
}
