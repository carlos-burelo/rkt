import { useNavigate } from "react-router";
import { ArrowRight, ChevronRight, RotateCw } from "lucide-react";
import { AppStatus, RktEvent, Deployment, AppMeta } from "../../shared/lib/types";
import { rel, dur } from "../../shared/lib/time";
import { Avatar, Dot, StateRail, StatusBadge } from "../../shared/components/ui";
import { Button } from "@/components/ui/button";
import { Metrics } from "./Metrics";
import { UsageStrip } from "./UsageStrip";
import { useUI } from "../../app/ui";
import { useFleet } from "./hooks";
import { PageHeader, Section, Stat } from "../../shared/components/patterns";
import { publicUrl, publicHost } from "../../shared/lib/url";

function FirstRun({ apps, metas, deps }: { apps: AppStatus[]; metas: AppMeta[]; deps: Deployment[] }) {
  const setCreateOpen = useUI((u) => u.setCreateOpen);
  const hasApp = apps.length > 0;
  const hasDomain = metas.some((m) => (m.domains ?? []).length > 0);
  const hasDeploy = deps.some((d) => d.status === "ok");
  const done = [hasApp, hasDomain, hasDeploy].filter(Boolean).length;
  if (done === 3) return null;
  const steps = [
    { ok: hasApp, label: "Crea tu primera app", hint: "apps add o Nueva app" },
    { ok: hasDomain, label: "Asigna un dominio", hint: "Dominios → nueva ruta" },
    { ok: hasDeploy, label: "Publica tu primer despliegue", hint: "Redesplegar desde la ficha" },
  ];
  return (
    <section className="border border-border bg-card/45">
      <header className="flex items-center gap-3 border-b border-border bg-secondary/20 px-3 py-2">
        <span aria-hidden className="h-1.5 w-1.5 bg-signal" />
        <h2 className="text-[12px] font-medium">Puesta en marcha</h2>
        <span className="flex-1" />
        <span className="tnum font-mono text-[10px] text-muted-foreground">{done}/3 listo</span>
      </header>
      <div className="grid gap-px bg-border md:grid-cols-3">
        {steps.map((s, i) => (
          <div key={s.label} className="bg-card/55 p-4">
            <div className="flex items-center justify-between gap-2">
              <span className="folio">0{i + 1}</span>
              <span className={s.ok ? "font-mono text-[10px] text-ok" : "font-mono text-[10px] text-muted-foreground"}>
                {s.ok ? "listo" : "pendiente"}
              </span>
            </div>
            <p className="mt-3 text-[13px] font-medium">{s.label}</p>
            <p className="mt-1 font-mono text-[10px] text-muted-foreground">{s.hint}</p>
          </div>
        ))}
      </div>
      {!hasApp && (
        <div className="border-t border-border px-3 py-2.5">
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            Crear mi primera app <ArrowRight className="h-3 w-3" />
          </Button>
        </div>
      )}
    </section>
  );
}

function Departures({ deps, onOpen }: { deps: Deployment[]; onOpen: (d: Deployment) => void }) {
  const recent = deps.slice(0, 6);
  return (
    <div className="divide-y divide-border/70">
      {recent.map((d) => (
        <button
          key={d.id}
          onClick={() => onOpen(d)}
          className="group flex w-full items-center gap-2.5 px-3 py-2 text-left hover:bg-accent/45"
        >
          <Dot status={d.status} />
          <span className="min-w-0 flex-1">
            <span className="flex items-baseline gap-1.5">
              <span className="truncate font-mono text-[11px]">{d.app}</span>
              <span className="tnum shrink-0 font-mono text-[9px] text-muted-foreground">#{d.id}</span>
            </span>
            <span className="mt-0.5 block truncate font-mono text-[9px] text-muted-foreground">
              {d.rev.slice(0, 8)} · {dur(d.started_at, d.finished_at)}
            </span>
          </span>
          <span className="tnum shrink-0 font-mono text-[9px] text-muted-foreground">
            {d.status === "running" ? "en curso" : rel(d.finished_at || d.started_at)}
          </span>
        </button>
      ))}
      {recent.length === 0 && <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">sin salidas</p>}
    </div>
  );
}

function Activity({ evs }: { evs: RktEvent[] }) {
  return (
    <div className="divide-y divide-border/60">
      {evs.slice(0, 8).map((e) => (
        <div key={e.id} className="grid grid-cols-[4.5rem_5rem_minmax(0,1fr)] items-baseline gap-2 px-3 py-1.5">
          <span className="tnum font-mono text-[9px] text-muted-foreground/60">{rel(e.ts)}</span>
          <span className="truncate font-mono text-[9px] tracking-[0.06em] text-muted-foreground uppercase">{e.kind}</span>
          <span className="min-w-0 truncate font-mono text-[10px] text-foreground/70">
            {e.app && <span className="text-foreground/90">{e.app}</span>} {e.msg}
          </span>
        </div>
      ))}
      {evs.length === 0 && <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">sin movimientos</p>}
    </div>
  );
}

export function Overview() {
  const { status, evs, deps, metas, loading } = useFleet();
  const navigate = useNavigate();
  const act = useUI((u) => u.act);
  const setSel = useUI((u) => u.setSel);
  const apps = status?.apps ?? [];
  const openApp = (name: string) => navigate(`/app/${name}`);

  if (loading) return <p className="font-mono text-[11px] text-muted-foreground">cargando manifiesto…</p>;

  const discovered = evs.filter((e) => e.kind === "discover").slice(0, 2);
  const openAlerts = evs.filter((e) => e.kind === "alert").slice(0, 3);
  const lastDep = (app: string) => deps.find((d) => d.app === app);
  const running = apps.filter((a) => a.running && !a.failed).length;
  const failed = apps.filter((a) => a.failed).length;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Resumen"
        description="Manifiesto de flota. Lo que está en marcha, lo que varó y lo que salió último; sin visitantes ni métricas que rkt no mida."
        meta={
          <span className="hidden items-center gap-2 font-mono text-[10px] text-muted-foreground sm:flex">
            <span className="h-1 w-1 bg-ok" />
            {running}/{apps.length} en marcha
            {failed > 0 && <span className="text-bad">· {failed} varado{failed > 1 ? "s" : ""}</span>}
          </span>
        }
      />

      {apps.length === 0 && <FirstRun apps={apps} metas={metas} deps={deps} />}
      <Metrics apps={apps} deps={deps} />

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_330px]">
        <div className="min-w-0 space-y-5">
          <section>
            <div className="mb-2 flex items-end justify-between gap-3">
              <div>
                <h2 className="text-[14px] font-medium tracking-tight">Unidades</h2>
                <p className="mt-0.5 font-mono text-[10px] text-muted-foreground">
                  app · puerto · destino · revisión
                </p>
              </div>
              <span className="tnum font-mono text-[10px] text-muted-foreground">{apps.length} en manifiesto</span>
            </div>
            <div className="border border-border" role="table" aria-label="manifiesto de apps">
              <div className="hidden grid-cols-[1.5rem_minmax(0,1.5fr)_5rem_minmax(0,1.5fr)_7rem_5.5rem_4.5rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 md:grid" aria-hidden>
                <span className="micro">est</span>
                <span className="micro">unidad</span>
                <span className="micro">puerto</span>
                <span className="micro">destino</span>
                <span className="micro">rev</span>
                <span className="micro text-right">hace</span>
                <span />
              </div>
              {apps.map((a, i) => {
                const meta = metas.find((m) => m.name === a.name);
                const domain = meta?.domains[0];
                const dep = lastDep(a.name);
                const state = a.failed ? "failed" : a.running ? "running" : "stopped";
                return (
                  <div
                    key={a.name}
                    onClick={() => openApp(a.name)}
                    className="group relative grid cursor-pointer grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-3 border-b border-border/70 bg-card/35 px-3 py-2.5 last:border-0 hover:bg-accent/40 md:grid-cols-[1.5rem_minmax(0,1.5fr)_5rem_minmax(0,1.5fr)_7rem_5.5rem_4.5rem]"
                    role="row"
                  >
                    <StateRail status={state} />
                    <Dot status={state} />
                    <div className="flex min-w-0 items-center gap-2.5">
                      <Avatar name={a.name} />
                      <div className="min-w-0">
                        <p className="truncate font-mono text-[12px] font-medium">{a.name}</p>
                        <p className="tnum truncate font-mono text-[9px] text-muted-foreground md:hidden">
                          {meta?.port ? `:${meta.port} · ` : ""}{domain ?? "sin dominio"}
                        </p>
                      </div>
                    </div>
                    <span className="tnum hidden font-mono text-[10px] text-muted-foreground md:block">
                      {meta?.port ? `:${meta.port}` : "—"}
                    </span>
                    <span className="hidden min-w-0 md:block">
                      {domain ? (
                        <a
                          href={publicUrl(domain, status)}
                          target="_blank"
                          rel="noreferrer"
                          onClick={(e) => e.stopPropagation()}
                          className="truncate font-mono text-[10px] text-foreground/75 underline-offset-4 hover:text-signal hover:underline"
                        >
                          {publicHost(domain, status)}
                        </a>
                      ) : (
                        <span className="font-mono text-[10px] text-muted-foreground">sin dominio</span>
                      )}
                    </span>
                    <span className="tnum hidden font-mono text-[10px] text-muted-foreground md:block">
                      {dep ? dep.rev.slice(0, 8) : "—"}
                    </span>
                    <span className="tnum hidden text-right font-mono text-[10px] text-muted-foreground md:block">
                      {dep ? rel(dep.finished_at || dep.started_at) : "—"}
                    </span>
                    <span className="flex items-center justify-end gap-1">
                      <StatusBadge status={state} className="md:hidden" />
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        className="opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
                        title={`restart ${a.name}`}
                        onClick={(e) => {
                          e.stopPropagation();
                          act(a.name, "restart");
                        }}
                      >
                        <RotateCw />
                      </Button>
                      <ChevronRight className="h-3.5 w-3.5 text-muted-foreground/40 transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" />
                    </span>
                  </div>
                );
              })}
              {apps.length === 0 && (
                <div className="px-4 py-8 text-center">
                  <p className="font-mono text-[11px] text-muted-foreground">terminal vacía</p>
                  <p className="mt-1 font-mono text-[10px] text-muted-foreground/70">
                    usa <span className="text-foreground">Nueva app</span> o <span className="text-foreground">rkt apps add</span>
                  </p>
                </div>
              )}
            </div>
          </section>

          <Section title="Movimientos" description="events · local" actions={<span className="tnum font-mono text-[9px] text-muted-foreground">{evs.length}</span>}>
            <Activity evs={evs} />
          </Section>
        </div>

        <aside className="min-w-0 space-y-5">
          <Section
            title="Salidas"
            description="últimos despliegues"
            actions={
              <button onClick={() => navigate("/deployments")} className="font-mono text-[9px] text-muted-foreground hover:text-foreground">
                ver todo
              </button>
            }
          >
            <Departures deps={deps} onOpen={setSel} />
          </Section>

          {openAlerts.length > 0 && (
            <Section title="Notas de despacho" description="alertas abiertas" accent>
              <div className="divide-y divide-border/60">
                {openAlerts.map((e) => (
                  <div key={e.id} className="flex items-start gap-2 px-1 py-2">
                    <span aria-hidden className="mt-1 h-1.5 w-1.5 shrink-0 bg-warn" />
                    <div className="min-w-0">
                      <p className="truncate font-mono text-[10px]"><span className="text-foreground">{e.app}</span> · {e.msg}</p>
                      <p className="mt-0.5 font-mono text-[9px] text-muted-foreground">{rel(e.ts)}</p>
                    </div>
                  </div>
                ))}
              </div>
            </Section>
          )}

          {discovered.length > 0 && (
            <Section title="Entrante" description="repos detectados">
              <div className="divide-y divide-border/60">
                {discovered.map((e) => (
                  <div key={e.id} className="flex items-center gap-2 px-1 py-2">
                    <div className="min-w-0 flex-1">
                      <p className="font-mono text-[10px]">nuevo repo</p>
                      <p className="truncate font-mono text-[9px] text-muted-foreground">{e.msg}</p>
                    </div>
                    <Button size="xs" variant="outline" onClick={() => openApp(e.app)}>importar</Button>
                  </div>
                ))}
              </div>
            </Section>
          )}

          <Section title="Medidor" description="access.log local · 30d">
            <UsageStrip />
          </Section>

          {apps.length > 0 && (
            <div className="grid grid-cols-2 gap-px border border-border bg-border">
              <Stat value={`${apps.length}`} label="unidades" sub="manifiesto" />
              <Stat value={`${metas.reduce((n, m) => n + (m.domains?.length ?? 0), 0)}`} label="destinos" sub="hosts en marcha" />
            </div>
          )}
        </aside>
      </div>
    </div>
  );
}
