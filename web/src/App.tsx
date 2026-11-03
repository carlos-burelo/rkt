import { useEffect, useRef, useState } from "react";
import {
  api, logout, me, Me, Status, Deployment, RktEvent, AppMeta,
  listApps, appAction, getEnv, listRoutes, getSettings, listUsers, deploymentLog, User,
} from "./api";
import { Login } from "./Login";
import { Dot, Err, Note } from "./components/ui";
import { DeployDrawer } from "./components/DeployDrawer";
import { Overview } from "./pages/Overview";
import { Deployments } from "./pages/Deployments";
import { AppDetail } from "./pages/AppDetail";
import { Logs } from "./pages/Logs";
import { Routes } from "./pages/Routes";
import { Settings } from "./pages/Settings";
import { Users } from "./pages/Users";

type Page = "overview" | "deployments" | "logs" | "routes" | "settings" | "users" | "app";

export function App() {
  const [who, setWho] = useState<Me | null>(null);
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
  const [note, setNote] = useState("");
  const [sel, setSel] = useState<Deployment | null>(null);
  const [buildLog, setBuildLog] = useState("");
  const [env, setEnvMap] = useState<Record<string, string>>({});
  const [routes, setRoutes] = useState<{ hosts: string[]; target: string }[]>([]);
  const [settings, setSettingsMap] = useState<Record<string, string>>({});
  const [users, setUsers] = useState<User[]>([]);
  const logRef = useRef<HTMLPreElement>(null);

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
        api<Status>("/api/v1/status"),
        api<Deployment[]>("/api/v1/deployments?n=50"),
        api<RktEvent[]>("/api/v1/events?n=30"),
        listApps(),
      ]);
      setStatus(st);
      setDeps(dp);
      setEvs(ev);
      setMetas(am);
      if (page === "logs") {
        const q = `/api/v1/logs?n=200${appFilter !== "" ? `&app=${appFilter}` : ""}`;
        setLog((await api<{ log: string }>(q)).log);
      }
      if (page === "routes") setRoutes(await listRoutes());
      if (page === "settings") setSettingsMap(await getSettings());
      if (page === "users" && isAdmin) setUsers(await listUsers());
      if (page === "app" && appFilter) setEnvMap(await getEnv(appFilter));
    } catch (e) {
      fail(e);
    }
  }

  useEffect(() => {
    me()
      .then(setWho)
      .catch(() => setWho(null));
  }, []);

  useEffect(() => {
    if (!authed) return;
    refresh();
    const t = setInterval(refresh, 5000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authed, page, appFilter]);

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
      setNote("");
      await appAction(app, a);
      setNote(`${a} ${app} ok`);
      refresh();
    } catch (e) {
      fail(e);
    }
  }

  function openApp(name: string) {
    setAppFilter(name);
    setPage("app");
  }

  const nav: { id: Page; label: string; admin?: boolean }[] = [
    { id: "overview", label: "Overview" },
    { id: "deployments", label: "Deployments" },
    { id: "logs", label: "Logs" },
    { id: "routes", label: "Domains" },
    { id: "settings", label: "Settings" },
    { id: "users", label: "Users", admin: true },
  ];

  return (
    <div className="flex min-h-screen bg-zinc-950 text-sm text-zinc-100">
      <aside className="flex w-56 shrink-0 flex-col border-r border-zinc-800 bg-zinc-900/40">
        <div className="border-b border-zinc-800 px-4 py-3 text-base font-bold tracking-tight">▲ rkt</div>
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
                onClick={() => setPage(n.id)}
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
        <div className="border-t border-zinc-800 p-3">
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

      <main className="min-w-0 flex-1 p-6">
        <div className="mx-auto max-w-4xl">
          <Err msg={err} />
          <Note msg={note} />
          {page === "overview" && <Overview apps={apps} evs={evs} openApp={openApp} />}
          {page === "deployments" && (
            <Deployments deps={vis} filtered={filter !== "" || appFilter !== ""} onSelect={setSel} />
          )}
          {page === "app" && (
            <AppDetail
              app={appFilter}
              meta={appMeta}
              env={env}
              deployments={deps.filter((d) => d.app === appFilter)}
              canWrite={canWrite}
              onAction={act}
              onOpenDeploy={setSel}
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
              follow={follow}
              setFollow={setFollow}
              refresh={refresh}
              logRef={logRef}
            />
          )}
          {page === "routes" && <Routes routes={routes} canWrite={canWrite} refresh={refresh} fail={fail} />}
          {page === "settings" && <Settings settings={settings} canWrite={canWrite} refresh={refresh} fail={fail} />}
          {page === "users" && isAdmin && <Users users={users} refresh={refresh} fail={fail} />}
        </div>
      </main>

      <DeployDrawer dep={sel} buildLog={buildLog} canWrite={canWrite} onAction={act} onClose={() => setSel(null)} />
    </div>
  );
}
