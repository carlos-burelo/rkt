import { useEffect, useRef, useState } from "react";
import { api } from "../shared/lib/http";
import { Me, Status, Deployment, RktEvent, AppMeta, User } from "../shared/lib/types";
import { logout, me } from "../features/auth/api";
import { Login } from "../features/auth/Login";
import { Dot, Err } from "../shared/components/ui";
import { Toast, ToastStack, CopyButton, Skeleton } from "../shared/components/feedback";
import { InstallButton } from "../shared/components/InstallButton";
import { CommandPalette } from "../shared/components/CommandPalette";
import { useOnline } from "../shared/hooks/useOnline";
import { DeployDrawer } from "../features/deployments/DeployDrawer";
import { Overview } from "../features/overview/Overview";
import { CreateDialog } from "../features/apps/CreateDialog";
import { AnalyticsPage } from "../features/overview/AnalyticsPage";
import { Observability } from "../features/overview/Observability";
import { Alerts } from "../features/alerts/Alerts";
import { EnvAll } from "../features/env/EnvAll";
import { CI } from "../features/ci/CI";
import { Deployments } from "../features/deployments/Deployments";
import { AppDetail } from "../features/apps/AppDetail";
import { Logs } from "../features/logs/Logs";
import { Routes } from "../features/routes/Routes";
import { Settings } from "../features/settings/Settings";
import { Users } from "../features/users/Users";
import { fetchStatus, fetchDeployments, fetchEvents } from "../features/overview/api";
import { deploymentLog, retryDeployment } from "../features/deployments/api";
import { listApps, appAction, getEnv } from "../features/apps/api";
import { fetchLog } from "../features/logs/api";
import { listRoutes } from "../features/routes/api";
import { getSettings } from "../features/settings/api";
import { listUsers } from "../features/users/api";

export type Page = "overview" | "deployments" | "logs" | "routes" | "settings" | "users" | "app" | "analytics" | "observability" | "environment" | "ci" | "alerts";

export function App() {
  const [who, setWho] = useState<Me | null>(null);
  const online = useOnline();
  const authed = who !== null;
  const isAdmin = who?.role === "admin";
  const canWrite = who?.role === "admin" || who?.role === "operator";
  const [page, setPage] = useState<Page>("deployments");
  const [filter, setFilter] = useState("");
  const [appFilter, setAppFilter] = useState("");
  const [status, setStatus] = useState<Status | null>(null);
  const [metas, setMetas] = useState<AppMeta[]>([]);
  const [deps, setDeps] = useState<Deployment[]>([]);
  const [evs, setEvs] = useState<RktEvent[]>([]);
  const [log, setLog] = useState("");
  const [follow, setFollow] = useState(true);
  const [err, setErr] = useState("");
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [boot, setBoot] = useState(true);
  const [sel, setSel] = useState<Deployment | null>(null);
  const [buildLog, setBuildLog] = useState("");
  const [env, setEnvMap] = useState<Record<string, string>>({});
  const [routes, setRoutes] = useState<{ hosts: string[]; target: string }[]>([]);
  const [settings, setSettingsMap] = useState<Record<string, string>>({});
  const [users, setUsers] = useState<User[]>([]);
  const [palette, setPalette] = useState(false);
  const [menu, setMenu] = useState(false);
  const [brand, setBrand] = useState("rkt");
  const [createOpen, setCreateOpen] = useState(false);
  const logRef = useRef<HTMLPreElement>(null);

  function toast(kind: "ok" | "err", msg: string) {
    const id = Date.now() + Math.random();
    setToasts((ts) => [...ts.slice(-3), { id, kind, msg }]);
  }

  function fail(e: unknown) {
    if (e instanceof Error && e.message.startsWith("no autorizado")) {
      setWho(null);
      return;
    }
    setErr(e instanceof Error ? e.message : "error");
  }

  async function refresh() {
    try {
      setErr("");
      const [st, dp, ev, am] = await Promise.all([
        fetchStatus(),
        fetchDeployments(),
        fetchEvents(),
        listApps(),
      ]);
      setStatus(st);
      setDeps(dp);
      setEvs(ev);
      setMetas(am);
      if (page === "logs") setLog(await fetchLog(appFilter));
      if (page === "routes") setRoutes(await listRoutes());
      if (page === "settings") setSettingsMap(await getSettings());
      if (page === "users" && isAdmin) setUsers(await listUsers());
      if (page === "app" && appFilter) setEnvMap(await getEnv(appFilter));
    } catch (e) {
      fail(e);
    } finally {
      setBoot(false);
    }
  }

  useEffect(() => {
    me()
      .then((m) => {
        setWho(m);
        getSettings()
          .then((s) => {
            if (s.site_name) setBrand(s.site_name);
          })
          .catch(() => {});
      })
      .catch(() => setWho(null));
  }, []);

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPalette((v) => !v);
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, []);

  useEffect(() => {
    if (!authed || !online) return;
    refresh();
    const t = setInterval(refresh, 5000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authed, online, page, appFilter]);

  useEffect(() => {
    if (follow && logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [log, follow]);

  useEffect(() => {
    if (sel) {
      setBuildLog("cargando…");
      deploymentLog(sel.id)
        .then(setBuildLog)
        .catch((e) => setBuildLog(e instanceof Error ? e.message : "sin log"));
    }
  }, [sel]);

  if (!authed) return <Login onOk={setWho} />;

  const apps = status?.apps ?? [];
  const f = filter.toLowerCase();
  const vis = deps.filter(
    (d) =>
      (appFilter === "" || d.app === appFilter) &&
      (f === "" || d.app.toLowerCase().includes(f) || d.rev.toLowerCase().includes(f)),
  );
  const appMeta = metas.find((m) => m.name === appFilter);

  async function act(app: string, a: "restart" | "deploy" | "rollback") {
    try {
      await appAction(app, a);
      toast("ok", `${a} ${app} ok`);
      refresh();
    } catch (e) {
      toast("err", e instanceof Error ? e.message : "error");
    }
  }

  function openApp(name: string) {
    setAppFilter(name);
    setPage("app");
    setMenu(false);
  }

  const nav: { id: Page; label: string; admin?: boolean }[] = [
    { id: "overview", label: "Overview" },
    { id: "deployments", label: "Deployments" },
    { id: "ci", label: "CI" },
    { id: "logs", label: "Logs" },
    { id: "analytics", label: "Analytics" },
    { id: "observability", label: "Observability" },
    { id: "alerts", label: "Alerts" },
    { id: "routes", label: "Domains" },
    { id: "environment", label: "Environment" },
    { id: "settings", label: "Settings" },
    { id: "users", label: "Users", admin: true },
  ];

  return (
    <div className="flex min-h-screen bg-zinc-950 text-sm text-zinc-100">
      {/* topbar móvil */}
      <div className="fixed inset-x-0 top-0 z-40 flex items-center gap-3 border-b border-zinc-800 bg-zinc-950/95 px-4 py-2.5 backdrop-blur md:hidden">
        <button onClick={() => setMenu(true)} className="rounded border border-zinc-800 px-2 py-1 text-zinc-300" aria-label="menú">
          ☰
        </button>
        <span className="font-bold tracking-tight">▲ {brand}</span>
        <span className="flex-1" />
        {!online && <span className="text-xs text-amber-300">offline</span>}
      </div>
      {menu && (
        <div className="fixed inset-0 z-40 bg-black/70 md:hidden" onClick={() => setMenu(false)} />
      )}
      <aside
        className={`fixed inset-y-0 left-0 z-50 flex w-64 shrink-0 flex-col border-r border-zinc-800 bg-zinc-950 transition-transform md:static md:z-auto md:w-56 md:translate-x-0 md:bg-zinc-900/40 ${
          menu ? "translate-x-0" : "-translate-x-full"
        }`}
      >
        <div className="flex items-center justify-between border-b border-zinc-800 px-4 py-3">
          <span className="text-base font-bold tracking-tight">▲ {brand}</span>
          <button onClick={() => setMenu(false)} className="rounded px-2 text-zinc-500 md:hidden" aria-label="cerrar">
            ✕
          </button>
        </div>
        {canWrite && (
          <div className="px-3 pt-3">
            <button
              onClick={() => setCreateOpen(true)}
              className="w-full rounded-md bg-zinc-100 px-2 py-1.5 text-xs font-medium text-zinc-900 hover:bg-white"
            >
              + Create New
            </button>
          </div>
        )}
        <div className="px-3 pt-3">
          <button
            onClick={() => setPalette(true)}
            className="flex w-full items-center justify-between rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1.5 text-xs text-zinc-500 hover:border-zinc-600 hover:text-zinc-300"
          >
            Buscar o accionar…
            <kbd className="rounded border border-zinc-700 px-1 font-mono text-[10px]">⌘K</kbd>
          </button>
        </div>
        <input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Buscar app o sha…"
          className="mx-3 mt-3 rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1.5 text-xs outline-none placeholder:text-zinc-600 focus:border-zinc-600"
        />
        <nav className="space-y-0.5 p-3">
          {nav
            .filter((n) => !n.admin || isAdmin)
            .map((n) => (
              <button
                key={n.id}
                onClick={() => {
                  setPage(n.id);
                  setMenu(false);
                }}
                className={`block w-full rounded-md px-2.5 py-1.5 text-left ${
                  page === n.id || (n.id === "deployments" && page === "app")
                    ? "bg-zinc-800 font-medium text-white"
                    : "text-zinc-400 hover:bg-zinc-800/50 hover:text-zinc-200"
                }`}
              >
                {n.label}
              </button>
            ))}
        </nav>
        <div className="border-t border-zinc-800 p-3">
          <p className="px-2.5 pb-1.5 text-[11px] uppercase tracking-wider text-zinc-600">Apps</p>
          <button
            onClick={() => setAppFilter("")}
            className={`block w-full rounded-md px-2.5 py-1.5 text-left font-mono text-xs ${
              appFilter === "" ? "bg-zinc-800 text-white" : "text-zinc-400 hover:text-zinc-200"
            }`}
          >
            todas
          </button>
          {apps.map((a) => (
            <button
              key={a.name}
              onClick={() => openApp(a.name)}
              className={`flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left font-mono text-xs ${
                appFilter === a.name ? "bg-zinc-800 text-white" : "text-zinc-400 hover:text-zinc-200"
              }`}
            >
              <Dot status={a.failed ? "failed" : a.running ? "running" : "stopped"} />
              {a.name}
            </button>
          ))}
        </div>
        <div className="flex-1" />
        <div className="space-y-1 border-t border-zinc-800 p-3">
          {!online && (
            <p className="rounded-md bg-amber-500/15 px-2.5 py-1.5 text-xs text-amber-300">
              Sin conexión — mostrando último estado
            </p>
          )}
          <InstallButton />
          <p className="px-2.5 font-mono text-[11px] text-zinc-600">
            {who?.user} · {who?.role} · v{status?.version ?? "…"}
          </p>
          <button
            onClick={() => {
              logout().finally(() => setWho(null));
            }}
            className="mt-1 block w-full rounded-md px-2.5 py-1.5 text-left text-xs text-zinc-500 hover:text-zinc-200"
          >
            Salir
          </button>
        </div>
      </aside>

      <main className="min-w-0 flex-1 p-4 pt-16 md:p-6 md:pt-6">
        <div className="mx-auto max-w-4xl">
          <div className="mb-4 hidden items-center gap-2 text-sm md:flex">
            <span className="text-zinc-500">▲ {brand}</span>
            <span className="text-zinc-700">/</span>
            <span className="capitalize">{page === "app" ? appFilter || "app" : page}</span>
            <span className="flex-1" />
            {!online && <span className="text-xs text-amber-300">offline</span>}
            {canWrite && (
              <button
                onClick={() => setCreateOpen(true)}
                className="rounded-md bg-zinc-100 px-2.5 py-1 text-xs font-medium text-zinc-900 hover:bg-white"
              >
                + Create New
              </button>
            )}
            <span className="font-mono text-[11px] text-zinc-600">{who?.user} · {who?.role}</span>
          </div>
          <Err msg={err} />
          {boot ? (
            <Skeleton rows={6} />
          ) : (
            <>
          {page === "overview" && <Overview apps={apps} evs={evs} deps={deps} metas={metas} openApp={openApp} />}
          {page === "deployments" && (
            <Deployments deps={vis} filtered={filter !== "" || appFilter !== ""} onSelect={setSel} />
          )}
          {page === "app" && (
            <AppDetail
              app={appFilter}
              meta={appMeta}
              status={apps.find((a) => a.name === appFilter)}
              env={env}
              deployments={deps.filter((d) => d.app === appFilter)}
              canWrite={canWrite}
              onAction={act}
              onOpenDeploy={setSel}
              onDeleted={() => {
                setAppFilter("");
                setPage("deployments");
                refresh();
              }}
              refresh={refresh}
              fail={fail}
            />
          )}
          {page === "logs" && (
            <Logs
              apps={apps}
              appFilter={appFilter}
              setAppFilter={setAppFilter}
              log={log}
              setLog={setLog}
              follow={follow}
              setFollow={setFollow}
              refresh={refresh}
              logRef={logRef}
              deps={deps}
            />
          )}
          {page === "routes" && <Routes routes={routes} canWrite={canWrite} refresh={refresh} fail={fail} />}
          {page === "settings" && <Settings settings={settings} canWrite={canWrite} refresh={refresh} fail={fail} />}
          {page === "users" && isAdmin && <Users users={users} refresh={refresh} fail={fail} />}
          {page === "analytics" && <AnalyticsPage />}
          {page === "observability" && <Observability />}
          {page === "alerts" && <Alerts canWrite={canWrite} fail={fail} />}
          {page === "environment" && <EnvAll fail={fail} />}
          {page === "ci" && <CI deps={deps} onSelect={setSel} />}
            </>
          )}
          <ToastStack toasts={toasts} dismiss={(id) => setToasts((ts) => ts.filter((t) => t.id !== id))} />
        </div>
      </main>

      <CreateDialog
        open={createOpen}
        setOpen={setCreateOpen}
        onCreated={(name) => {
          toast("ok", `app ${name} creada`);
          refresh();
          openApp(name);
        }}
        goDomains={() => setPage("routes")}
        fail={fail}
      />
      <DeployDrawer
        dep={sel}
        buildLog={buildLog}
        canWrite={canWrite}
        onAction={act}
        onRetry={(id) => {
          retryDeployment(id)
            .then((rev) => {
              toast("ok", `retry ok ${rev.slice(0, 8)}`);
              setSel(null);
              refresh();
            })
            .catch((e) => toast("err", e instanceof Error ? e.message : "error"));
        }}
        onClose={() => setSel(null)}
      />
      <CommandPalette
        open={palette}
        setOpen={setPalette}
        apps={apps}
        metas={metas}
        deps={deps}
        goPage={(p) => setPage(p)}
        openApp={openApp}
        openDeploy={setSel}
        onAction={act}
      />
    </div>
  );
}
