import { useNavigate } from "react-router";
import { rel } from "../../shared/lib/time";
import { Avatar, Dot } from "../../shared/components/ui";
import { Metrics } from "./Metrics";
import { UsageStrip } from "./UsageStrip";
import { useFleet } from "./hooks";

export function Overview() {
  const { status, evs, deps, metas, loading } = useFleet();
  const navigate = useNavigate();
  const openApp = (name: string) => navigate(`/app/${name}`);
  const apps = status?.apps ?? [];

  if (loading) return <p className="text-sm text-muted-foreground">cargando…</p>;

  const discovered = evs.filter((e) => e.kind === "discover").slice(0, 3);
  const openAlerts = evs.filter((e) => e.kind === "alert").slice(0, 3);
  return (
    <div className="space-y-6">
      <Metrics apps={apps} deps={deps} />
      {openAlerts.length > 0 && (
        <section>
          <h3 className="mb-2 text-sm font-medium">Alerts</h3>
          {openAlerts.map((e) => (
            <div key={e.id} className="mb-2 flex items-center gap-3 rounded-lg border border-amber-900 bg-amber-950/30 px-4 py-3">
              <span className="h-2 w-2 rounded-full bg-amber-400" />
              <div className="flex-1">
                <p className="font-mono text-xs">{e.app} · {e.msg}</p>
                <p className="font-mono text-[11px] text-zinc-500">{rel(e.ts)}</p>
              </div>
            </div>
          ))}
        </section>
      )}
      {discovered.length > 0 && (
        <section>
          <h3 className="mb-2 text-sm font-medium">Inbox</h3>
          {discovered.map((e) => (
            <div key={e.id} className="mb-2 flex items-center gap-3 rounded-lg border border-zinc-700 bg-zinc-900 px-4 py-3">
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
        </section>
      )}
      <section>
        <h3 className="mb-2 text-sm font-medium">Projects</h3>
        <div className="overflow-hidden rounded-lg border border-zinc-800">
          {apps.map((a) => {
            const domain = metas.find((m) => m.name === a.name)?.domains[0];
            return (
              <button
                key={a.name}
                onClick={() => openApp(a.name)}
                className="flex w-full items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-3 text-left last:border-0 hover:bg-zinc-800/40"
              >
                <Avatar name={a.name} />
                <div className="min-w-0">
                  <p className="font-mono text-sm">{a.name}</p>
                  <p className="truncate font-mono text-xs text-zinc-500">{domain ?? "sin dominio"}</p>
                </div>
                <span className="flex-1" />
                <span className="hidden text-xs text-zinc-500 sm:block">
                  {a.failed ? "failed" : a.running ? "running" : "stopped"} · {a.restarts} restarts
                </span>
                <Dot status={a.failed ? "failed" : a.running ? "running" : "stopped"} />
                <span className="text-zinc-600">›</span>
              </button>
            );
          })}
          {apps.length === 0 && <p className="p-6 text-sm text-zinc-500">Sin apps. Registra con <span className="font-mono">apps add</span>.</p>}
        </div>
      </section>
      {evs.length > 0 && (
        <section>
          <h3 className="mb-2 text-sm font-medium">Recent activity</h3>
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
            {evs.slice(0, 8).map((e) => (
              <p key={e.id} className="truncate py-0.5 text-xs text-zinc-400">
                <span className="font-mono text-zinc-600">{rel(e.ts)}</span>{" "}
                <span className="font-mono text-zinc-300">{e.kind}</span>{" "}
                <span className="font-mono">{e.app}</span> {e.msg}
              </p>
            ))}
          </div>
        </section>
      )}
      <UsageStrip />
    </div>
  );
}
