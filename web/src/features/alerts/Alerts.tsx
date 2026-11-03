import { useEffect, useState } from "react";
import { listRules, addRule, delRule, AlertRule } from "./api";

export function Alerts({ canWrite, fail }: { canWrite: boolean; fail: (e: unknown) => void }) {
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [app, setApp] = useState("*");
  const [kind, setKind] = useState("failed");
  const [threshold, setThreshold] = useState("1");
  const [windowMin, setWindowMin] = useState("60");

  function load() {
    listRules().then(setRules).catch(fail);
  }

  useEffect(load, []);

  return (
    <div className="space-y-3">
      <div className="overflow-hidden rounded-lg border border-zinc-800">
        {rules.map((r) => (
          <div key={r.id} className="flex items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-2.5 last:border-0">
            <span className="font-mono text-xs">#{r.id}</span>
            <span className="font-mono text-xs">{r.app}</span>
            <span className="rounded-full bg-zinc-800 px-2 py-0.5 font-mono text-[11px] text-zinc-300">{r.kind}</span>
            <span className="font-mono text-xs text-zinc-400">≥{r.threshold}/{r.window_min}min</span>
            <span className="flex-1" />
            {r.last_fired ? (
              <span className="font-mono text-[11px] text-amber-300">disparó {r.last_fired.slice(0, 16).replace("T", " ")}</span>
            ) : (
              <span className="font-mono text-[11px] text-zinc-600">sin disparos</span>
            )}
            {canWrite && (
              <button onClick={() => delRule(r.id).then(load).catch(fail)}
                className="text-xs text-zinc-600 hover:text-red-300">
                borrar
              </button>
            )}
          </div>
        ))}
        {rules.length === 0 && <p className="p-6 text-sm text-zinc-500">Sin reglas. Crea una abajo.</p>}
      </div>
      {canWrite && (
        <div className="flex flex-wrap gap-2">
          <input value={app} onChange={(e) => setApp(e.target.value)} placeholder="app o *"
            className="w-32 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
          <select value={kind} onChange={(e) => setKind(e.target.value)}
            className="rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none">
            <option value="failed">failed</option>
            <option value="error_rate">error_rate</option>
          </select>
          <input value={threshold} onChange={(e) => setThreshold(e.target.value)} placeholder={kind === "failed" ? "fallos ≥" : "tasa ≥ (0.1)"}
            className="w-32 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
          <input value={windowMin} onChange={(e) => setWindowMin(e.target.value)} placeholder="ventana min"
            className="w-28 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
          <button
            onClick={() => {
              addRule(app.trim() || "*", kind, parseFloat(threshold), parseInt(windowMin, 10))
                .then(load)
                .catch(fail);
            }}
            className="rounded border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800"
          >
            crear
          </button>
        </div>
      )}
    </div>
  );
}
