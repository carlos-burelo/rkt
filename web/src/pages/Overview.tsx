import { AppStatus, RktEvent } from "../api";
import { rel } from "../time";
import { Dot } from "../components/ui";

export function Overview({
  apps,
  evs,
  openApp,
}: {
  apps: AppStatus[];
  evs: RktEvent[];
  openApp: (name: string) => void;
}) {
  return (
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
  );
}
