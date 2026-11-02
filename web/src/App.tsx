import { useEffect, useState } from "react";
import { api, getToken, clearToken, Status, Deployment, RktEvent } from "./api";
import { Login } from "./Login";

function Pill({ ok, text }: { ok: boolean; text: string }) {
  return (
    <span
      className={`inline-block rounded-full px-2 py-0.5 text-xs ${
        ok ? "bg-emerald-500/15 text-emerald-300" : "bg-red-500/15 text-red-300"
      }`}
    >
      ● {text}
    </span>
  );
}

export function App() {
  const [authed, setAuthed] = useState(getToken() !== "");
  const [tab, setTab] = useState<"status" | "deployments" | "events" | "logs">("status");
  const [status, setStatus] = useState<Status | null>(null);
  const [deps, setDeps] = useState<Deployment[]>([]);
  const [evs, setEvs] = useState<RktEvent[]>([]);
  const [log, setLog] = useState("");
  const [logApp, setLogApp] = useState("");
  const [err, setErr] = useState("");

  async function refresh() {
    try {
      setErr("");
      if (tab === "status") setStatus(await api<Status>("/api/v1/status"));
      if (tab === "deployments") setDeps(await api<Deployment[]>("/api/v1/deployments?n=50"));
      if (tab === "events") setEvs(await api<RktEvent[]>("/api/v1/events?n=50"));
      if (tab === "logs")
        setLog(
          (await api<{ log: string }>(`/api/v1/logs?n=200${logApp !== "" ? `&app=${logApp}` : ""}`)).log,
        );
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
  }, [authed, tab, logApp]);

  if (!authed) return <Login onOk={() => setAuthed(true)} />;

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100">
      <header className="flex items-center gap-4 border-b border-zinc-800 px-4 py-2.5">
        <span className="font-semibold">rkt</span>
        {(["status", "deployments", "events", "logs"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded px-2 py-1 text-sm ${tab === t ? "bg-zinc-800 text-white" : "text-zinc-400 hover:text-zinc-200"}`}
          >
            {t}
          </button>
        ))}
        <span className="flex-1" />
        <button
          onClick={() => {
            clearToken();
            setAuthed(false);
          }}
          className="text-sm text-zinc-400 hover:text-zinc-200"
        >
          salir
        </button>
      </header>
      <main className="mx-auto max-w-5xl p-4">
        {err !== "" && <p className="mb-3 text-sm text-red-400">{err}</p>}
        {tab === "status" && status && (
          <div className="space-y-2">
            <p className="text-sm text-zinc-400">
              versión {status.version} · {status.routes} routes · {status.apps.length} apps
            </p>
            {status.apps.map((a) => (
              <div key={a.name} className="flex items-center gap-3 rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2">
                <span className="font-mono text-sm">{a.name}</span>
                <Pill ok={a.running && !a.failed} text={a.failed ? "failed" : a.running ? "running" : "stopped"} />
                <span className="text-xs text-zinc-500">restarts {a.restarts}</span>
              </div>
            ))}
            {status.apps.length === 0 && <p className="text-sm text-zinc-500">sin apps</p>}
          </div>
        )}
        {tab === "deployments" && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-zinc-500">
                <th className="py-1">id</th>
                <th>app</th>
                <th>rev</th>
                <th>status</th>
                <th>fin</th>
              </tr>
            </thead>
            <tbody>
              {deps.map((d) => (
                <tr key={d.id} className="border-t border-zinc-800">
                  <td className="py-1.5 font-mono">{d.id}</td>
                  <td className="font-mono">{d.app}</td>
                  <td className="font-mono text-zinc-400">{d.rev.slice(0, 8)}</td>
                  <td>{d.status}</td>
                  <td className="text-zinc-500">{d.finished_at}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {tab === "events" && (
          <div className="space-y-1.5">
            {evs.map((e) => (
              <div key={e.id} className="rounded border border-zinc-800 bg-zinc-900 px-3 py-1.5 text-sm">
                <span className="font-mono text-zinc-500">{e.ts}</span>{" "}
                <span className="font-mono text-zinc-300">{e.kind}</span>{" "}
                <span className="font-mono">{e.app}</span>{" "}
                <span className="text-zinc-400">{e.msg}</span>
              </div>
            ))}
            {evs.length === 0 && <p className="text-sm text-zinc-500">sin eventos</p>}
          </div>
        )}
        {tab === "logs" && (
          <div className="space-y-2">
            <input
              value={logApp}
              onChange={(e) => setLogApp(e.target.value)}
              placeholder="app (vacío = operativo)"
              className="w-64 rounded border border-zinc-700 bg-zinc-900 px-2 py-1 text-sm outline-none focus:border-zinc-500"
            />
            <pre className="overflow-auto rounded-lg border border-zinc-800 bg-black p-3 font-mono text-xs leading-relaxed text-zinc-300">
              {log}
            </pre>
          </div>
        )}
      </main>
    </div>
  );
}
