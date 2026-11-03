import { useNavigate } from "react-router";
import { ArrowRight } from "lucide-react";
import { AppStatus, RktEvent, Deployment, AppMeta } from "../../shared/lib/types";
import { rel } from "../../shared/lib/time";
import { Avatar, StateRail, StatusBadge } from "../../shared/components/ui";
import { Button } from "@/components/ui/button";
import { Metrics } from "./Metrics";
import { UsageStrip } from "./UsageStrip";
import { useUI } from "../../app/ui";
import { useFleet } from "./hooks";

function FirstRun({ apps, metas, deps }: { apps: AppStatus[]; metas: AppMeta[]; deps: Deployment[] }) {
  const setCreateOpen = useUI((u) => u.setCreateOpen);
  const hasApp = apps.length > 0;
  const hasDomain = metas.some((m) => (m.domains ?? []).length > 0);
  const hasDeploy = deps.some((d) => d.status === "ok");
  const done = [hasApp, hasDomain, hasDeploy].filter(Boolean).length;
  if (done === 3) return null;
  return (
    <section className="overflow-hidden rounded-xl border bg-card">
      <div aria-hidden className="manifest-strip h-1 w-full opacity-60" />
      <div className="p-5">
      <div className="mb-1 flex items-center justify-between">
        <h3 className="text-sm font-medium">Primeros pasos</h3>
        <span className="tnum font-mono text-xs text-muted-foreground">{done}/3</span>
      </div>
      <div className="mb-3 h-1.5 overflow-hidden rounded bg-secondary">
        <div className="h-full rounded bg-primary transition-all" style={{ width: `${(done / 3) * 100}%` }} />
      </div>
      {(
        [
          [hasApp, "Crea tu primera app"],
          [hasDomain, "Asigna un dominio"],
          [hasDeploy, "Publica tu primer deploy"],
        ] as const
      ).map(([ok, label]) => (
        <p key={label} className="flex items-center gap-2 py-1 text-xs">
          <span className={`inline-block h-2 w-2 rounded-full ${ok ? "bg-emerald-400" : "bg-zinc-700"}`} />
          <span className={ok ? "text-muted-foreground line-through" : ""}>{label}</span>
        </p>
      ))}
      {!hasApp && (
        <Button className="mt-3" size="sm" onClick={() => setCreateOpen(true)}>
          Crear mi primera app
        </Button>
      )}
      </div>
    </section>
  );
}

export function Overview() {
  const { status, evs, deps, metas, loading } = useFleet();
  const navigate = useNavigate();
  const act = useUI((u) => u.act);
  const setSel = useUI((u) => u.setSel);
  const apps = status?.apps ?? [];
  const openApp = (name: string) => navigate(`/app/${name}`);

  if (loading) return <p className="text-sm text-muted-foreground">cargando…</p>;

  const discovered = evs.filter((e) => e.kind === "discover").slice(0, 3);
  const openAlerts = evs.filter((e) => e.kind === "alert").slice(0, 3);
  const lastDep = (app: string) => deps.find((d) => d.app === app);

  return (
    <div className="space-y-6">
      {apps.length === 0 && <FirstRun apps={apps} metas={metas} deps={deps} />}
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
            <div key={e.id} className="mb-2 flex items-center gap-3 rounded-lg border px-4 py-3">
              <div className="flex-1">
                <p className="text-sm font-medium">New repository detected</p>
                <p className="font-mono text-xs text-muted-foreground">{e.msg}</p>
              </div>
              <Button onClick={() => openApp(e.app)}>Import</Button>
            </div>
          ))}
        </section>
      )}
      <section>
        <h3 className="mb-2 text-sm font-medium">Projects</h3>
        <div className="overflow-hidden rounded-lg border">
          {apps.map((a) => {
            const meta = metas.find((m) => m.name === a.name);
            const domain = meta?.domains[0];
            const dep = lastDep(a.name);
            const state = a.failed ? "failed" : a.running ? "running" : "stopped";
            return (
              <div
                key={a.name}
                className="relative flex w-full items-center gap-3 border-b bg-card/40 py-3 pl-4 pr-4 text-left last:border-0"
              >
                <StateRail status={state} />
                <Avatar name={a.name} />
                <div className="min-w-0">
                  <button onClick={() => openApp(a.name)} className="font-mono text-sm hover:underline">
                    {a.name}
                  </button>
                  <p className="tnum truncate font-mono text-[11px] text-muted-foreground">
                    {meta?.port ? `bahía :${meta.port} · ` : ""}{domain ?? "sin dominio"}
                  </p>
                </div>
                <span className="tnum hidden font-mono text-[11px] text-muted-foreground md:block">
                  {dep ? `${dep.rev.slice(0, 8)} · ${rel(dep.finished_at || dep.started_at)}` : "sin deploys"}
                </span>
                <span className="flex-1" />
                <StatusBadge status={state} />
                <Button
                  variant="ghost" size="sm"
                  className="text-xs"
                  onClick={() => act(a.name, "restart")}
                  title={`restart ${a.name}`}
                >
                  restart
                </Button>
                <Button
                  variant="ghost" size="sm"
                  className="inline-flex items-center gap-1 text-xs"
                  onClick={() => openApp(a.name)}
                >
                  abrir <ArrowRight className="h-3 w-3" />
                </Button>
              </div>
            );
          })}
          {apps.length === 0 && (
            <p className="p-6 text-sm text-muted-foreground">
              Sin apps. Usa <span className="font-medium text-foreground">Create New</span> arriba o <span className="font-mono">apps add</span> en CLI.
            </p>
          )}
        </div>
      </section>
      {evs.length > 0 && (
        <section>
          <h3 className="mb-2 text-sm font-medium">Recent activity</h3>
          <div className="rounded-lg border bg-card p-4">
            {evs.slice(0, 8).map((e) => (
              <p key={e.id} className="truncate py-0.5 font-mono text-xs text-muted-foreground">
                <span className="text-zinc-600">{rel(e.ts)}</span>{" "}
                <span className="text-zinc-300">{e.kind}</span> {e.app} {e.msg}
              </p>
            ))}
          </div>
        </section>
      )}
      <UsageStrip />
    </div>
  );
}
