import { useEffect, useRef, useState } from "react";
import { Root, Backdrop, Popup, Title, Close } from "@base_ui/react/dialog";
import {
  api, logout, me, Me, Status, Deployment, RktEvent, AppMeta, User,
  listApps, appAction, getEnv, setEnv, delEnv, listRoutes, addRoute, delRoute,
  getSettings, setSetting, listUsers, createUser, deleteUser, deploymentLog,
} from "./api";
import { Login } from "./Login";
import { rel, dur } from "./time";

type Page = "overview" | "deployments" | "logs" | "routes" | "settings" | "users" | "app";

function Dot({ status }: { status: string }) {
  const ok = status === "ok" || status === "running";
  const run = status === "running";
  return (
    <span
      className={`inline-block h-2 w-2 rounded-full ${ok ? "bg-emerald-400" : "bg-red-400"} ${run ? "animate-pulse" : ""}`}
      title={status}
    />
  );
}

function Err({ msg }: { msg: string }) {
  if (!msg) return null;
  return <p className="mb-4 rounded-md border border-red-900 bg-red-950/50 px-3 py-2 text-xs text-red-300">{msg}</p>;
}

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
  const [envKey, setEnvKey] = useState("");
  const [envVal, setEnvVal] = useState("");
  const [routes, setRoutes] = useState<{ hosts: string[]; target: string }[]>([]);
  const [newHosts, setNewHosts] = useState("");
  const [newTarget, setNewTarget] = useState("");
  const [settings, setSettingsMap] = useState<Record<string, string>>({});
  const [users, setUsers] = useState<User[]>([]);
  const [nu, setNu] = useState({ name: "", password: "", role: "viewer" });
  const logRef = useRef<HTMLPreElement>(null);

  function fail(e: unknown, setAuthedCheck = true) {
    if (e instanceof Error && e.message.startsWith("no autorizado") && setAuthedCheck) {
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
      setErr(e instanceof Error ? e.message : "error");
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
          {note !== "" && (
            <p className="mb-4 rounded-md border border-emerald-900 bg-emerald-950/50 px-3 py-2 text-xs text-emerald-300">{note}</p>
          )}

          {page === "overview" && (
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              {apps.map((a) => (
                <button
                  key={a.name}
                  onClick={() => openApp(a.name)}
                  className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4 text-left hover:border-zinc-600"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-mono font-medium">{a.name}</span>
                    <Dot status={a.failed ? "failed" : a.running ? "running" : "stopped"} />
                  </div>
                  <p className="mt-2 text-xs text-zinc-500">
                    {a.failed ? "failed" : a.running ? "running" : "stopped"} · {a.restarts} restarts
                  </p>
                </button>
              ))}
              {apps.length === 0 && <p className="text-sm text-zinc-500">Sin apps. Registra con <span className="font-mono">apps add</span>.</p>}
              {evs.length > 0 && (
                <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4 md:col-span-2">
                  <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">Actividad reciente</p>
                  {evs.slice(0, 8).map((e) => (
                    <p key={e.id} className="truncate py-0.5 text-xs text-zinc-400">
                      <span className="font-mono text-zinc-600">{rel(e.ts)}</span>{" "}
                      <span className="font-mono text-zinc-300">{e.kind}</span>{" "}
                      <span className="font-mono">{e.app}</span> {e.msg}
                    </p>
                  ))}
                </div>
              )}
            </div>
          )}

          {page === "deployments" && (
            <div className="overflow-hidden rounded-lg border border-zinc-800">
              {vis.map((d) => (
                <button
                  key={d.id}
                  onClick={() => setSel(d)}
                  className="flex w-full items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-3 text-left last:border-0 hover:bg-zinc-800/40"
                >
                  <Dot status={d.status} />
                  <span className="font-mono">{d.app}</span>
                  <span className="font-mono text-xs text-zinc-500">{d.rev.slice(0, 8)}</span>
                  <span className="flex-1" />
                  <span className="text-xs text-zinc-500">
                    {d.status === "running" ? "building…" : rel(d.finished_at || d.started_at)}
                  </span>
                  <span className="font-mono text-xs text-zinc-600">#{d.id}</span>
                </button>
              ))}
              {vis.length === 0 && <p className="p-6 text-sm text-zinc-500">Sin deployments{filter || appFilter ? " para este filtro" : ""}.</p>}
            </div>
          )}

          {page === "app" && (
            <div className="space-y-4">
              <div className="flex items-center gap-3">
                <h2 className="font-mono text-lg">{appFilter || "(sin app)"}</h2>
                <span className="flex-1" />
                {canWrite && appFilter && (
                  <div className="flex gap-2">
                    {(["restart", "deploy", "rollback"] as const).map((a) => (
                      <button
                        key={a}
                        onClick={() => act(appFilter, a)}
                        className="rounded-md border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800"
                      >
                        {a}
                      </button>
                    ))}
                  </div>
                )}
              </div>
              {appMeta && (
                <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4 font-mono text-xs">
                  <p className="text-zinc-400">{appMeta.command} {(appMeta.args ?? []).join(" ")}</p>
                  <p className="mt-1 text-zinc-500">{appMeta.dir}</p>
                  {appMeta.deploy && (
                    <p className="mt-1 text-zinc-500">
                      {appMeta.deploy.repo} @ {appMeta.deploy.ref}
                      {appMeta.deploy.auto ? " · auto" : ""}
                    </p>
                  )}
                </div>
              )}
              <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
                <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">Environment (restart requerido)</p>
                {Object.entries(env).map(([k, v]) => (
                  <div key={k} className="flex items-center gap-2 py-1 font-mono text-xs">
                    <span className="w-48 truncate text-zinc-200">{k}</span>
                    <span className="flex-1 truncate text-zinc-500">{v}</span>
                    {canWrite && (
                      <button onClick={() => delEnv(appFilter, k).then(refresh).catch((e) => fail(e))} className="text-zinc-600 hover:text-red-300">
                        borrar
                      </button>
                    )}
                  </div>
                ))}
                {Object.keys(env).length === 0 && <p className="text-xs text-zinc-600">Sin vars inline.</p>}
                {canWrite && (
                  <div className="mt-2 flex gap-2">
                    <input value={envKey} onChange={(e) => setEnvKey(e.target.value)} placeholder="CLAVE"
                      className="w-40 rounded border border-zinc-700 bg-zinc-900 px-2 py-1 font-mono text-xs outline-none" />
                    <input value={envVal} onChange={(e) => setEnvVal(e.target.value)} placeholder="valor"
                      className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1 font-mono text-xs outline-none" />
                    <button
                      onClick={() => {
                        setEnv(appFilter, envKey.trim(), envVal).then(() => {
                          setEnvKey("");
                          setEnvVal("");
                          refresh();
                        }).catch((e) => fail(e));
                      }}
                      className="rounded border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800"
                    >
                      guardar
                    </button>
                  </div>
                )}
              </div>
              <div className="overflow-hidden rounded-lg border border-zinc-800">
                {deps.filter((d) => d.app === appFilter).slice(0, 10).map((d) => (
                  <button key={d.id} onClick={() => setSel(d)}
                    className="flex w-full items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-2.5 text-left last:border-0 hover:bg-zinc-800/40">
                    <Dot status={d.status} />
                    <span className="font-mono text-xs text-zinc-400">{d.rev.slice(0, 8)}</span>
                    <span className="flex-1" />
                    <span className="text-xs text-zinc-500">{dur(d.started_at, d.finished_at)}</span>
                    <span className="font-mono text-xs text-zinc-600">#{d.id}</span>
                  </button>
                ))}
              </div>
            </div>
          )}

          {page === "logs" && (
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <select
                  value={appFilter}
                  onChange={(e) => setAppFilter(e.target.value)}
                  className="rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none"
                >
                  <option value="">operativo (rkt.log)</option>
                  {apps.map((a) => (
                    <option key={a.name} value={a.name}>
                      {a.name}
                    </option>
                  ))}
                </select>
                <label className="flex items-center gap-1.5 text-xs text-zinc-400">
                  <input type="checkbox" checked={follow} onChange={(e) => setFollow(e.target.checked)} />
                  seguir
                </label>
                <button onClick={refresh} className="rounded-md border border-zinc-800 px-2 py-1 text-xs text-zinc-400 hover:text-zinc-100">
                  recargar
                </button>
              </div>
              <pre ref={logRef} className="h-[60vh] overflow-auto rounded-lg border border-zinc-800 bg-black p-3 font-mono text-xs leading-relaxed text-zinc-300">
                {log || "sin líneas"}
              </pre>
            </div>
          )}

          {page === "routes" && (
            <div className="space-y-3">
              <div className="overflow-hidden rounded-lg border border-zinc-800">
                {routes.map((r) => (
                  <div key={r.hosts.join(",") + r.target} className="flex items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-2.5 last:border-0">
                    <span className="font-mono text-xs">{r.hosts.join(", ")}</span>
                    <span className="text-zinc-600">→</span>
                    <span className="font-mono text-xs text-zinc-400">{r.target}</span>
                    <span className="flex-1" />
                    {canWrite && (
                      <button
                        onClick={() => delRoute(r.hosts[0]).then(refresh).catch((e) => fail(e))}
                        className="text-xs text-zinc-600 hover:text-red-300"
                      >
                        borrar
                      </button>
                    )}
                  </div>
                ))}
                {routes.length === 0 && <p className="p-6 text-sm text-zinc-500">Sin routes.</p>}
              </div>
              {canWrite && (
                <div className="flex gap-2">
                  <input value={newHosts} onChange={(e) => setNewHosts(e.target.value)} placeholder="host1, host2"
                    className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
                  <input value={newTarget} onChange={(e) => setNewTarget(e.target.value)} placeholder="http://127.0.0.1:3000"
                    className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
                  <button
                    onClick={() => {
                      addRoute(newHosts.split(",").map((s) => s.trim()).filter(Boolean), newTarget.trim())
                        .then(() => {
                          setNewHosts("");
                          setNewTarget("");
                          refresh();
                        }).catch((e) => fail(e));
                    }}
                    className="rounded border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800"
                  >
                    agregar
                  </button>
                </div>
              )}
            </div>
          )}

          {page === "settings" && (
            <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
              {Object.entries(settings).map(([k, v]) => (
                <div key={k} className="flex items-center gap-2 py-1.5">
                  <span className="w-40 font-mono text-xs text-zinc-300">{k}</span>
                  <input
                    defaultValue={v}
                    id={`set-${k}`}
                    disabled={!canWrite}
                    className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1 font-mono text-xs outline-none disabled:text-zinc-500"
                  />
                  {canWrite && (
                    <button
                      onClick={() => {
                        const el = document.getElementById(`set-${k}`) as HTMLInputElement;
                        setSetting(k, el.value).then(refresh).catch((e) => fail(e));
                      }}
                      className="rounded border border-zinc-700 px-2 py-1 text-xs hover:bg-zinc-800"
                    >
                      guardar
                    </button>
                  )}
                </div>
              ))}
            </div>
          )}

          {page === "users" && isAdmin && (
            <div className="space-y-3">
              <div className="overflow-hidden rounded-lg border border-zinc-800">
                {users.map((u) => (
                  <div key={u.name} className="flex items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-2.5 last:border-0">
                    <span className="font-mono text-xs">{u.name}</span>
                    <span className="rounded-full bg-zinc-800 px-2 py-0.5 font-mono text-[11px] text-zinc-300">{u.role}</span>
                    <span className="flex-1" />
                    <span className="font-mono text-[11px] text-zinc-600">{u.created_at}</span>
                    <button onClick={() => deleteUser(u.name).then(refresh).catch((e) => fail(e))}
                      className="text-xs text-zinc-600 hover:text-red-300">
                      borrar
                    </button>
                  </div>
                ))}
              </div>
              <div className="flex gap-2">
                <input value={nu.name} onChange={(e) => setNu({ ...nu, name: e.target.value })} placeholder="usuario"
                  className="w-32 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
                <input value={nu.password} onChange={(e) => setNu({ ...nu, password: e.target.value })} placeholder="password (mín 12)" type="password"
                  className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
                <select value={nu.role} onChange={(e) => setNu({ ...nu, role: e.target.value })}
                  className="rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none">
                  <option value="viewer">viewer</option>
                  <option value="operator">operator</option>
                  <option value="admin">admin</option>
                </select>
                <button
                  onClick={() => {
                    createUser(nu.name.trim(), nu.password, nu.role)
                      .then(() => {
                        setNu({ name: "", password: "", role: "viewer" });
                        refresh();
                      }).catch((e) => fail(e));
                  }}
                  className="rounded border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800"
                >
                  crear
                </button>
              </div>
            </div>
          )}
        </div>
      </main>

      <Root
        open={sel !== null}
        onOpenChange={(open: boolean) => {
          if (!open) setSel(null);
        }}
      >
        <Backdrop className="fixed inset-0 bg-black/70" />
        <Popup className="fixed right-0 top-0 flex h-full w-full max-w-xl flex-col border-l border-zinc-800 bg-zinc-950">
          {sel && (
            <div className="flex h-full flex-col space-y-4 overflow-hidden p-6">
              <div className="flex items-center gap-2">
                <Dot status={sel.status} />
                <Title className="font-mono text-base">
                  {sel.app} <span className="text-zinc-500">#{sel.id}</span>
                </Title>
                <span className="flex-1" />
                <Close className="rounded border border-zinc-800 px-2 py-0.5 text-xs text-zinc-400 hover:text-zinc-100">
                  cerrar
                </Close>
              </div>
              <dl className="grid grid-cols-2 gap-1.5 font-mono text-xs">
                <dt className="text-zinc-500">status</dt><dd>{sel.status}</dd>
                <dt className="text-zinc-500">rev</dt><dd className="truncate">{sel.rev}</dd>
                <dt className="text-zinc-500">inicio</dt><dd>{rel(sel.started_at)}</dd>
                <dt className="text-zinc-500">duración</dt><dd>{dur(sel.started_at, sel.finished_at)}</dd>
              </dl>
              {canWrite && (
                <div className="flex gap-2">
                  {(["restart", "deploy", "rollback"] as const).map((a) => (
                    <button key={a} onClick={() => act(sel.app, a)}
                      className="rounded-md border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800">
                      {a} {sel.app}
                    </button>
                  ))}
                </div>
              )}
              <p className="text-xs uppercase tracking-wider text-zinc-500">build log</p>
              <pre className="min-h-0 flex-1 overflow-auto rounded-lg border border-zinc-800 bg-black p-3 font-mono text-xs leading-relaxed text-zinc-300">
                {buildLog}
              </pre>
            </div>
          )}
        </Popup>
      </Root>
    </div>
  );
}
