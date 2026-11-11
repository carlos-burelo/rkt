import { useNavigate } from "react-router";
import { ChevronRight, RotateCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { Avatar, Dot, MicroLink, StateRail, StatusBadge, stateOf } from "../../shared/components/ui";
import { Section, Stat } from "../../shared/components/patterns";
import { rel } from "../../shared/lib/time";
import { stateLabel } from "../../shared/lib/labels";
import { publicHost, publicUrl } from "../../shared/lib/url";
import { useUI } from "../../app/ui";
import { useFleet } from "./hooks";
import { useI18n } from "../../shared/i18n";
import { Departures } from "./Departures";
import { UsageStrip } from "./UsageStrip";
import { TechIcon } from "../../shared/components/TechIcon";
import { stackOfApp } from "../apps/model";
import type { AppMeta, AppStatus, Deployment, Status } from "../../shared/lib/types";

// Tabla de unidades: estado, dominio, puerto, rev. Lee fleet + actions.
export function UnitsTable({
  apps,
  metas,
  deps,
  status,
}: {
  apps: AppStatus[];
  metas: AppMeta[];
  deps: Deployment[];
  status: Status | null;
}) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const act = useUI((u) => u.act);
  const openApp = (name: string) => navigate(`/app/${name}`);
  const lastDep = (app: string) => deps.find((d) => d.app === app);
  const rank = { failed: 0, degraded: 1, running: 2, ok: 2, stopped: 3 } as const;
  const sortedApps = [...apps].sort((a, b) => rank[stateOf(a)] - rank[stateOf(b)]);
  return (
    <section>
      <div className="mb-2 flex flex-wrap items-end justify-between gap-x-3 gap-y-1">
        <div>
          <h2 className="text-[14px] font-medium tracking-tight">{t("overview.units.title")}</h2>
          <p className="mt-0.5 font-mono text-[10px] text-muted-foreground">
            {t("overview.units.columns")}
          </p>
        </div>
        <span className="tnum font-mono text-[10px] text-muted-foreground">
          {t("overview.units.count", { n: apps.length })}
        </span>
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
          const stack = meta ? stackOfApp(meta) : "unknown";
          const showStack = stack !== "unknown" && stack !== "external";
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
                {showStack && <TechIcon kind={stack} size={18} />}
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
  );
}

// Sidebar del resumen: salidas, alertas, descubrimientos, uso, stats.
export function OverviewSide({
  deps,
  openAlerts,
  discovered,
  openApp,
}: {
  deps: Deployment[];
  openAlerts: { id: number; app: string; msg: string; ts: string }[];
  discovered: { id: number; app: string; msg: string }[];
  openApp: (n: string) => void;
}) {
  const { t } = useI18n();
  const navigate = useNavigate();
  return (
    <aside className="min-w-0 space-y-5">
      <Section
        title={t("overview.sections.departures")}
        description={t("overview.sections.departuresDesc")}
        actions={<MicroLink to="/deployments" className="text-[10px]">{t("overview.sections.seeAll")}</MicroLink>}
      >
        <Departures deps={deps} />
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

      <StatsStrip />
    </aside>
  );
}

function StatsStrip() {
  const { t } = useI18n();
  const { status, metas } = useFleet();
  const apps = status?.apps ?? [];
  if (apps.length === 0) return null;
  return (
    <div className="grid grid-cols-2 gap-px border border-border bg-border">
      <Stat value={`${apps.length}`} label={t("overview.stats.units")} sub={t("overview.stats.unitsSub")} />
      <Stat value={`${metas.reduce((n, m) => n + (m.domains?.length ?? 0), 0)}`} label={t("overview.stats.targets")} sub={t("overview.stats.targetsSub")} />
    </div>
  );
}
