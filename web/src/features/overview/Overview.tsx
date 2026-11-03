import { AppStatus, RktEvent, Deployment } from "../../shared/lib/types";
import { rel } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { Metrics } from "./Metrics";
import { UsageStrip } from "./UsageStrip";

export function Overview({
  apps,
  evs,
  deps,
  openApp,
}: {
  apps: AppStatus[];
  evs: RktEvent[];
  deps: Deployment[];
  openApp: (name: string) => void;
}) {
  return (
    <div className="space-y-3">
      <Metrics apps={apps} deps={deps} />
      <UsageStrip />
      {evs.filter((e) => e.kind === "discover").slice(0, 3).map((e) => (
        <div key={e.id} className="flex items-center gap-3 rounded-lg border border-zinc-700 bg-zinc-900 px-4 py-3">
          <div className="flex-1">
            <p className="text-sm font-medium">New repository detected</p>
            <p className="font-mono text-xs text-zinc-500">{e.msg}</p>
          </div>
          <button
            onClick={() => openApp(e.app)}
            className="rounded-md bg-zinc-100 px-3 py-1.5 text-xs font-medium text-zinc-900 hover:bg-white"
          >
            Import
          </button>
        </div>
      ))}
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
    </div>
  );
}
