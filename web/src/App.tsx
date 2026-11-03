import { useEffect, useRef, useState } from "react";
import { Root, Backdrop, Popup, Title, Close } from "@base_ui/react/dialog";
import { api, getToken, clearToken, Status, Deployment, RktEvent } from "./api";
import { Login } from "./Login";
import { rel, dur } from "./time";

type Page = "overview" | "deployments" | "logs";

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

export function App() {
  const [authed, setAuthed] = useState(getToken() !== "");
  const [page, setPage] = useState<Page>("deployments");
  const [filter, setFilter] = useState("");
  const [appFilter, setAppFilter] = useState("");
  const [status, setStatus] = useState<Status | null>(null);
  const [deps, setDeps] = useState<Deployment[]>([]);
  const [evs, setEvs] = useState<RktEvent[]>([]);
  const [log, setLog] = useState("");
  const [follow, setFollow] = useState(true);
  const [err, setErr] = useState("");
  const [sel, setSel] = useState<Deployment | null>(null);
  const logRef = useRef<HTMLPreElement>(null);

  async function refresh() {
    try {
      setErr("");
      const [st, dp, ev] = await Promise.all([
        api<Status>("/api/v1/status"),
        api<Deployment[]>("/api/v1/deployments?n=50"),
        api<RktEvent[]>("/api/v1/events?n=30"),
      ]);
      setStatus(st);
      setDeps(dp);
      setEvs(ev);
      if (page === "logs") {
        const q = `/api/v1/logs?n=200${appFilter !== "" ? `&app=${appFilter}` : ""}`;
        setLog((await api<{ log: string }>(q)).log);
      }
    } catch (e) {
      if (e instanceof Error && e.message.startsWith("no autorizado")) {
        setAuthed(false);
        return;
      }
      setErr(e instanceof Error ? e.message : "error");
    }
  }

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

  if (!authed) return <Login onOk={() => setAuthed(true)} />;

  const apps = status?.apps ?? [];
  const f = filter.toLowerCase();
  const vis = deps.filter(
    (d) =>
      (appFilter === "" || d.app === appFilter) &&
      (f === "" || d.app.toLowerCase().includes(f) || d.rev.toLowerCase().includes(f)),
  );

  return (
    <div className="flex min-h-screen bg-zinc-950 text-sm text-zinc-100">
      {/* sidebar */}
      <aside className="flex w-56 shrink-0 flex-col border-r border-zinc-800 bg-zinc-900/40">
        <div className="border-b border-zinc-800 px-4 py-3 text-base font-bold tracking-tight">▲ rkt</div>
        <input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Buscar app o sha…"
          className="mx-3 mt-3 rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1.5 text-xs outline-none placeholder:text-zinc-600 focus:border-zinc-600"
        />
        <nav className="space-y-0.5 p-3">
          {(["overview", "deployments", "logs"] as const).map((p) => (
            <button
              key={p}
              onClick={() => setPage(p)}
              className={`block w-full rounded-md px-2.5 py-1.5 text-left capitalize ${
                page === p ? "bg-zinc-800 font-medium text-white" : "text-zinc-400 hover:bg-zinc-800/50 hover:text-zinc-200"
              }`}
            >
              {p === "overview" ? "Overview" : p === "deployments" ? "Deployments" : "Logs"}
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
              onClick={() => {
                setAppFilter(a.name);
                setPage("deployments");
              }}
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
          <p className="px-2.5 font-mono text-[11px] text-zinc-600">v{status?.version ?? "…"}</p>
          <button
            onClick={() => {
              clearToken();
              setAuthed(false);
            }}
            className="mt-1 block w-full rounded-md px-2.5 py-1.5 text-left text-xs text-zinc-500 hover:text-zinc-200"
          >
            Salir
          </button>
        </div>
      </aside>

      {/* main */}
      <main className="min-w-0 flex-1 p-6">
        <div className="mx-auto max-w-4xl">
          {err !== "" && (
            <p className="mb-4 rounded-md border border-red-900 bg-red-950/50 px-3 py-2 text-xs text-red-300">{err}</p>
          )}

          {page === "overview" && (
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              {apps.map((a) => (
                <button
                  key={a.name}
                  onClick={() => {
                    setAppFilter(a.name);
                    setPage("deployments");
                  }}
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
              <pre
                ref={logRef}
                className="h-[60vh] overflow-auto rounded-lg border border-zinc-800 bg-black p-3 font-mono text-xs leading-relaxed text-zinc-300"
              >
                {log || "sin líneas"}
              </pre>
            </div>
          )}
        </div>
      </main>

      {/* detalle deployment */}
      <Root
        open={sel !== null}
        onOpenChange={(open: boolean) => {
          if (!open) setSel(null);
        }}
      >
        <Backdrop className="fixed inset-0 bg-black/70" />
        <Popup className="fixed right-0 top-0 h-full w-full max-w-md overflow-auto border-l border-zinc-800 bg-zinc-950 p-6">
          {sel && (
            <div className="space-y-4">
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
              <dl className="space-y-1.5 font-mono text-xs">
                <div className="flex justify-between"><dt className="text-zinc-500">status</dt><dd>{sel.status}</dd></div>
                <div className="flex justify-between"><dt className="text-zinc-500">rev</dt><dd>{sel.rev}</dd></div>
                <div className="flex justify-between"><dt className="text-zinc-500">inicio</dt><dd>{rel(sel.started_at)}</dd></div>
                <div className="flex justify-between"><dt className="text-zinc-500">duración</dt><dd>{dur(sel.started_at, sel.finished_at)}</dd></div>
              </dl>
              <button
                onClick={() => {
                  setAppFilter(sel.app);
                  setSel(null);
                  setPage("logs");
                }}
                className="w-full rounded-md border border-zinc-700 px-2 py-1.5 text-xs hover:bg-zinc-800"
              >
                ver logs de {sel.app}
              </button>
            </div>
          )}
        </Popup>
      </Root>
    </div>
  );
}
