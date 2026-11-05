import { Link, useNavigate } from "react-router";
import { ArrowRight, ChevronRight, RotateCw } from "lucide-react";
import { AppStatus, RktEvent, Deployment, AppMeta } from "../../shared/lib/types";
import { rel, dur } from "../../shared/lib/time";
import { Avatar, Dot, StateRail, StatusBadge, stateOf } from "../../shared/components/ui";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { stateLabel } from "../../shared/lib/labels";
import { Metrics } from "./Metrics";
import { UsageStrip } from "./UsageStrip";
import { useUI } from "../../app/ui";
import { useFleet } from "./hooks";
import { PageHeader, Section, Stat } from "../../shared/components/patterns";
import { publicUrl, publicHost } from "../../shared/lib/url";
import { useI18n } from "../../shared/i18n";

function FirstRun({ apps, metas, deps }: { apps: AppStatus[]; metas: AppMeta[]; deps: Deployment[] }) {
  const { t } = useI18n();
  const setCreateOpen = useUI((u) => u.setCreateOpen);
  const hasApp = apps.length > 0;
  const hasDomain = metas.some((m) => (m.domains ?? []).length > 0);
  const hasDeploy = deps.some((d) => d.status === "ok");
  const done = [hasApp, hasDomain, hasDeploy].filter(Boolean).length;
  if (done === 3) return null;
  const steps = [
    { ok: hasApp, label: t("overview.firstRun.stepApp"), hint: t("overview.firstRun.stepAppHint") },
    { ok: hasDomain, label: t("overview.firstRun.stepDomain"), hint: t("overview.firstRun.stepDomainHint") },
    { ok: hasDeploy, label: t("overview.firstRun.stepDeploy"), hint: t("overview.firstRun.stepDeployHint") },
  ];
  return (
    <section className="border border-border bg-card/45">
      <header className="flex items-center gap-3 border-b border-border bg-secondary/20 px-3 py-2">
        <span aria-hidden className="h-1.5 w-1.5 bg-signal" />
        <h2 className="text-[12px] font-medium">{t("overview.firstRun.title")}</h2>
        <span className="flex-1" />
        <span className="tnum font-mono text-[10px] text-muted-foreground">
          {t("overview.firstRun.progress", { done })}
        </span>
      </header>
      <div className="grid gap-px bg-border md:grid-cols-3">
        {steps.map((s, i) => (
          <div key={s.label} className="bg-card/55 p-4">
            <div className="flex items-center justify-between gap-2">
              <span className="folio">0{i + 1}</span>
              <span className={s.ok ? "font-mono text-[10px] text-ok" : "font-mono text-[10px] text-muted-foreground"}>
                {s.ok ? t("overview.firstRun.ready") : t("overview.firstRun.pending")}
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
            {t("overview.firstRun.create")} <ArrowRight className="h-3 w-3" />
          </Button>
        </div>
      )}
    </section>
  );
}

function Departures({ deps, onOpen }: { deps: Deployment[]; onOpen: (d: Deployment) => void }) {
  const { t } = useI18n();
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
              <span className="truncate font-mono text-[12px]">{d.app}</span>
              <span className="tnum shrink-0 font-mono text-[10px] text-muted-foreground">#{d.id}</span>
            </span>
            <span className="mt-0.5 block truncate font-mono text-[10px] text-muted-foreground">
              {d.rev.slice(0, 8)} · {dur(d.started_at, d.finished_at)}
            </span>
          </span>
          <span className="tnum shrink-0 font-mono text-[10px] text-muted-foreground">
            {d.status === "running" ? t("deploy.running") : rel(d.finished_at || d.started_at)}
          </span>
        </button>
      ))}
      {recent.length === 0 && (
        <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">{t("overview.departures.empty")}</p>
      )}
    </div>
  );
}

function Activity({ evs }: { evs: RktEvent[] }) {
  const { t } = useI18n();
  return (
    <div className="divide-y divide-border/60">
      {evs.slice(0, 8).map((e) => (
        <div key={e.id} className="grid grid-cols-[4.5rem_5rem_minmax(0,1fr)] items-baseline gap-2 px-3 py-1.5">
          <span className="tnum font-mono text-[10px] text-muted-foreground/60">{rel(e.ts)}</span>
          <span className="truncate font-mono text-[10px] tracking-[0.06em] text-muted-foreground uppercase">{e.kind}</span>
          <span className="min-w-0 truncate font-mono text-[11px] text-foreground/70">
            {e.app && <span className="text-foreground/90">{e.app}</span>} {e.msg}
          </span>
        </div>
      ))}
      {evs.length === 0 && (
        <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">{t("overview.activity.empty")}</p>
      )}
    </div>
  );
}

export function Overview() {
  const { t, tn } = useI18n();
  const { status, evs, deps, metas, loading } = useFleet();
  const navigate = useNavigate();
  const act = useUI((u) => u.act);
  const setSel = useUI((u) => u.setSel);
  const apps = status?.apps ?? [];
  const openApp = (name: string) => navigate(`/app/${name}`);

  if (loading)
    return <p className="font-mono text-[11px] text-muted-foreground">{t("overview.loading")}</p>;

  const discovered = evs.filter((e) => e.kind === "discover").slice(0, 2);
  const openAlerts = evs.filter((e) => e.kind === "alert").slice(0, 3);
  const lastDep = (app: string) => deps.find((d) => d.app === app);
  const running = apps.filter((a) => a.running && !a.failed).length;
  const failed = apps.filter((a) => a.failed).length;
  const rank = { failed: 0, degraded: 1, running: 2, ok: 2, stopped: 3 } as const;
  const sortedApps = [...apps].sort((a, b) => rank[stateOf(a)] - rank[stateOf(b)]);
  const failedApps = sortedApps.filter((a) => a.failed);
  const degradedApps = sortedApps.filter((a) => stateOf(a) === "degraded");
  const attentionApps = [...failedApps, ...degradedApps];
  const attention = attentionApps.length > 0;

  return (
    <div className="space-y-5">
      <PageHeader
        title={t("nav.overview")}
        description={t("overview.description")}
        meta={
          <span className="hidden items-center gap-2 font-mono text-[10px] text-muted-foreground lg:flex">
            <span className="h-1 w-1 bg-ok" />
            {t("overview.meta.running", { running, total: apps.length })}
            {failed > 0 && <span className="text-bad">· {tn("overview.stuck", failed)}</span>}
          </span>
        }
      />

      {attention && (
        <section
          aria-label={t("overview.attention.aria")}
          className={cn(
            "flex flex-wrap items-center gap-x-3 gap-y-2 border px-3 py-2",
            failedApps.length > 0
              ? "border-bad/30 bg-bad/[0.055]"
              : "border-warn/30 bg-warn/[0.05]",
          )}
        >
          <p className="flex min-w-0 flex-1 basis-full items-center gap-2 font-mono text-[11px] leading-relaxed sm:basis-0">
            <Dot status={failedApps.length > 0 ? "failed" : "degraded"} />
            <span className="min-w-0">
              {failedApps.length > 0 && (
                <span className="text-bad">{tn("overview.stuckFem", failedApps.length)}</span>
              )}
              {failedApps.length > 0 && degradedApps.length > 0 && (
                <span className="text-muted-foreground"> · </span>
              )}
              {degradedApps.length > 0 && (
                <span className="text-warn">{tn("overview.degraded", degradedApps.length)}</span>
              )}
              <span className="text-muted-foreground"> · {attentionApps.map((a) => a.name).join(", ")}</span>
            </span>
          </p>
          <div className="flex shrink-0 items-center gap-3">
            <Button size="xs" variant="outline" onClick={() => openApp(attentionApps[0].name)}>
              {t("overview.attention.open")}
            </Button>
            <Link
              to="/alerts"
              className="inline-flex items-center gap-1 font-mono text-[10px] tracking-[0.08em] text-muted-foreground uppercase hover:text-foreground"
            >
              {t("overview.attention.alerts")} <ArrowRight className="h-3 w-3" />
            </Link>
          </div>
        </section>
      )}

      {apps.length === 0 && <FirstRun apps={apps} metas={metas} deps={deps} />}
      <Metrics apps={apps} deps={deps} />

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_330px]">
        <div className="min-w-0 space-y-5">
          <section>
            <div className="mb-2 flex flex-wrap items-end justify-between gap-x-3 gap-y-1">
              <div>
                <h2 className="text-[14px] font-medium tracking-tight">{t("overview.units.title")}</h2>
                <p className="mt-0.5 font-mono text-[10px] text-muted-foreground">
                  {t("overview.units.columns")}
                </p>
              </div>
              <div className="flex items-center gap-2.5">
                {attention && (
                  <span className={cn("font-mono text-[10px]", failedApps.length > 0 ? "text-bad" : "text-warn")}>
                    {t("overview.units.attentionFirst")}
                  </span>
                )}
                <span className="tnum font-mono text-[10px] text-muted-foreground">
                  {t("overview.units.count", { n: apps.length })}
                </span>
              </div>
            </div>
            <div className="border border-border" role="table" aria-label={t("overview.units.tableAria")}>
              <div role="row" className="hidden grid-cols-[3.5rem_minmax(0,1.5fr)_5rem_minmax(0,1.5fr)_7rem_5.5rem_4.5rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 lg:grid">
                <span role="columnheader" className="micro">{t("overview.col.state")}</span>
                <span role="columnheader" className="micro">{t("overview.col.unit")}</span>
                <span role="columnheader" className="micro">{t("overview.col.port")}</span>
                <span role="columnheader" className="micro">{t("overview.col.target")}</span>
                <span role="columnheader" className="micro">{t("overview.col.rev")}</span>
                <span role="columnheader" className="micro text-right">{t("overview.col.ago")}</span>
                <span role="columnheader" className="sr-only">{t("overview.col.actions")}</span>
              </div>
              {sortedApps.map((a) => {
                const meta = metas.find((m) => m.name === a.name);
                const domain = meta?.domains[0];
                const dep = lastDep(a.name);
                const state = stateOf(a);
                return (
                  <div
                    key={a.name}
                    onClick={() => openApp(a.name)}
                    onKeyDown={(e) => {
                      if (e.target !== e.currentTarget) return;
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        openApp(a.name);
                      }
                    }}
                    tabIndex={0}
                    aria-label={t("overview.units.open", { name: a.name })}
                    className={cn(
                      "group relative grid cursor-pointer grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-3 border-b border-border/70 bg-card/35 px-3 py-2.5 last:border-0 hover:bg-accent/40 focus-visible:bg-accent/40 lg:grid-cols-[3.5rem_minmax(0,1.5fr)_5rem_minmax(0,1.5fr)_7rem_5.5rem_4.5rem]",
                      state === "failed" && "bg-bad/[0.045] hover:bg-bad/[0.075] focus-visible:bg-bad/[0.075]",
                    )}
                    role="row"
                  >
                    <StateRail status={state} />
                    <span role="cell" className="flex items-center">
                      <Dot status={state} />
                    </span>
                    <div role="cell" className="flex min-w-0 items-center gap-2.5">
                      <Avatar name={a.name} />
                      <div className="min-w-0">
                        <p className="flex min-w-0 items-baseline gap-1.5 font-mono text-[13px]">
                          <span className="truncate font-medium">{a.name}</span>
                          <span
                            className={cn(
                              "hidden shrink-0 text-[10px] tracking-[0.04em] uppercase sm:inline",
                              state === "failed"
                                ? "text-bad"
                                : state === "degraded"
                                  ? "text-warn"
                                  : state === "stopped"
                                    ? "text-muted-foreground/70"
                                    : "text-ok/80",
                            )}
                          >
                            {stateLabel(state)}
                          </span>
                        </p>
                        <p className="tnum truncate font-mono text-[10px] text-muted-foreground lg:hidden">
                          {meta?.port ? `:${meta.port} · ` : ""}{domain ?? t("overview.units.noDomain")}
                        </p>
                      </div>
                    </div>
                    <span role="cell" className="tnum hidden font-mono text-[11px] text-muted-foreground lg:block">
                      {meta?.port ? `:${meta.port}` : "—"}
                    </span>
                    <span role="cell" className="hidden min-w-0 lg:block">
                      {domain ? (
                        <a
                          href={publicUrl(domain, status)}
                          target="_blank"
                          rel="noreferrer"
                          onClick={(e) => e.stopPropagation()}
                          className="truncate font-mono text-[11px] text-foreground/75 underline-offset-4 hover:text-signal hover:underline"
                        >
                          {publicHost(domain, status)}
                        </a>
                      ) : (
                        <span className="font-mono text-[11px] text-muted-foreground">{t("overview.units.noDomain")}</span>
                      )}
                    </span>
                    <span role="cell" className="tnum hidden font-mono text-[11px] text-muted-foreground lg:block">
                      {dep ? dep.rev.slice(0, 8) : "—"}
                    </span>
                    <span role="cell" className="tnum hidden text-right font-mono text-[11px] text-muted-foreground lg:block">
                      {dep ? rel(dep.finished_at || dep.started_at) : "—"}
                    </span>
                    <span role="cell" className="flex items-center justify-end gap-1">
                      <StatusBadge status={state} className="sm:hidden" />
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        className={cn(
                          "opacity-100 transition-opacity lg:opacity-0 lg:group-hover:opacity-100 lg:group-focus-within:opacity-100 max-lg:size-8",
                          state === "failed" && "lg:opacity-100",
                        )}
                        title={t("overview.units.restart", { name: a.name })}
                        aria-label={t("overview.units.restart", { name: a.name })}
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
                  <p className="font-mono text-[11px] text-muted-foreground">{t("overview.units.empty")}</p>
                  <p className="mt-1 font-mono text-[10px] text-muted-foreground/70">
                    {t("overview.units.emptyHintPrefix")}{" "}
                    <span className="text-foreground">{t("shell.newApp")}</span>{" "}
                    {t("overview.units.emptyHintJoin")}{" "}
                    <span className="text-foreground">rkt apps add</span>
                  </p>
                </div>
              )}
            </div>
          </section>

          <Section title={t("overview.sections.events")} description={t("overview.sections.eventsDesc")} actions={<span className="tnum font-mono text-[10px] text-muted-foreground">{evs.length}</span>}>
            <Activity evs={evs} />
          </Section>
        </div>

        <aside className="min-w-0 space-y-5">
          <Section
            title={t("overview.sections.departures")}
            description={t("overview.sections.departuresDesc")}
            actions={
              <button onClick={() => navigate("/deployments")} className="inline-flex items-center gap-1 font-mono text-[10px] text-muted-foreground hover:text-foreground">
                {t("overview.sections.seeAll")} <ArrowRight className="h-3 w-3" />
              </button>
            }
          >
            <Departures deps={deps} onOpen={setSel} />
          </Section>

          {openAlerts.length > 0 && (
            <Section title={t("overview.sections.alerts")} description={t("overview.sections.alertsDesc")} accent>
              <div className="divide-y divide-border/60">
                {openAlerts.map((e) => (
                  <div key={e.id} className="flex items-start gap-2 px-1 py-2">
                    <span aria-hidden className="mt-1 h-1.5 w-1.5 shrink-0 bg-warn" />
                    <div className="min-w-0">
                      <p className="truncate font-mono text-[10px]"><span className="text-foreground">{e.app}</span> · {e.msg}</p>
                      <p className="mt-0.5 font-mono text-[10px] text-muted-foreground">{rel(e.ts)}</p>
                    </div>
                  </div>
                ))}
              </div>
            </Section>
          )}

          {discovered.length > 0 && (
            <Section title={t("overview.sections.incoming")} description={t("overview.sections.incomingDesc")}>
              <div className="divide-y divide-border/60">
                {discovered.map((e) => (
                  <div key={e.id} className="flex items-center gap-2 px-1 py-2">
                    <div className="min-w-0 flex-1">
                      <p className="font-mono text-[10px]">{t("overview.incoming.newRepo")}</p>
                      <p className="truncate font-mono text-[10px] text-muted-foreground">{e.msg}</p>
                    </div>
                    <Button size="xs" variant="outline" onClick={() => openApp(e.app)}>{t("overview.incoming.import")}</Button>
                  </div>
                ))}
              </div>
            </Section>
          )}

          <Section title={t("overview.sections.meter")} description={t("overview.sections.meterDesc")}>
            <UsageStrip />
          </Section>

          {apps.length > 0 && (
            <div className="grid grid-cols-2 gap-px border border-border bg-border">
              <Stat value={`${apps.length}`} label={t("overview.stats.units")} sub={t("overview.stats.unitsSub")} />
              <Stat value={`${metas.reduce((n, m) => n + (m.domains?.length ?? 0), 0)}`} label={t("overview.stats.targets")} sub={t("overview.stats.targetsSub")} />
            </div>
          )}
        </aside>
      </div>
    </div>
  );
}
