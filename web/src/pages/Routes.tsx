import { useState } from "react";
import { Route, addRoute, delRoute } from "../api";

export function Routes({
  routes,
  canWrite,
  refresh,
  fail,
}: {
  routes: Route[];
  canWrite: boolean;
  refresh: () => void;
  fail: (e: unknown) => void;
}) {
  const [hosts, setHosts] = useState("");
  const [target, setTarget] = useState("");

  return (
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
                onClick={() => delRoute(r.hosts[0]).then(refresh).catch(fail)}
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
          <input value={hosts} onChange={(e) => setHosts(e.target.value)} placeholder="host1, host2"
            className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
          <input value={target} onChange={(e) => setTarget(e.target.value)} placeholder="http://127.0.0.1:3000"
            className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
          <button
            onClick={() => {
              addRoute(hosts.split(",").map((s) => s.trim()).filter(Boolean), target.trim())
                .then(() => {
                  setHosts("");
                  setTarget("");
                  refresh();
                }).catch(fail);
            }}
            className="rounded border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800"
          >
            agregar
          </button>
        </div>
      )}
    </div>
  );
}
