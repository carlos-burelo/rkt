import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { ArrowRight, ExternalLink } from "lucide-react";
import { dur, rel } from "../../shared/lib/time";
import { CopyButton } from "../../shared/components/feedback";
import { Section, Stat, Time } from "../../shared/components/patterns";
import { Dot } from "../../shared/components/ui";
import { deployStateLabel, eventKindLabel } from "../../shared/lib/labels";
import { fetchAnalytics, Analytics } from "../overview/api";
import { useFleet } from "../overview/hooks";
import { useEnvCount } from "./hooks";
import { publicUrl, publicHost } from "../../shared/lib/url";
import { AppPreview } from "./AppPreview";
import { useI18n } from "../../shared/i18n";
import { useUI } from "../../app/ui";

function Check({ ok, label, hint }: { ok: boolean; label: string; hint: string }) {
  const { t } = useI18n();
  return (
    <div className="flex items-center gap-2.5 py-2">
      <span
        aria-hidden
        className={`h-2 w-2 shrink-0 border ${ok ? "border-ok/60 bg-ok" : "border-border bg-transparent"}`}
      />
      <span className="min-w-0 flex-1">
        <span className="block text-[12px]">{label}</span>
        <span className="mt-0.5 block truncate font-mono text-[10px] text-muted-foreground">{hint}</span>
      </span>
      <span className={`font-mono text-[10px] tracking-[0.08em] uppercase ${ok ? "text-ok" : "text-muted-foreground/70"}`}>
        {ok ? t("apps.overview.check.ready") : t("apps.overview.check.pending")}
      </span>
    </div>
  );
}

function Cell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0 bg-card/45 px-3 py-2">
      <p className="micro">{label}</p>
      <div className="tnum mt-1 truncate font-mono text-[12px]">{children}</div>
    </div>
  );
}

export function AppOverview() {
  const { t } = useI18n();
  const { name = "" } = useParams();
  const { metas, deps, evs, status } = useFleet();
  const setSel = useUI((u) => u.setSel);
  const [stats, setStats] = useState<Analytics | null>(null);

  const meta = metas.find((m) => m.name === name);
  const st = status?.apps.find((a) => a.name === name);
  const appDeps = deps.filter((d) => d.app === name);
  const appEvents = evs.filter((e) => e.app === name);
  const prod = appDeps.find((d) => d.status === "ok");
  const domains = meta?.domains ?? [];
  const running = st ? st.running && !st.failed : false;
  const envCount = useEnvCount(name);

  useEffect(() => {
    if (domains[0]) {
      fetchAnalytics(domains[0], 7).then(setStats).catch(() => setStats(null));
    } else {
      setStats(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name, domains.join(",")]);

  if (meta?.external) {
    const cmd = meta.run_command || "pnpm dev";
    return (
      <div className="space-y-4">
        <Section
          title={t("apps.overview.externalTitle")}
          description={t("apps.overview.externalDesc")}
          accent={!running}
        >
          <div className="grid grid-cols-2 gap-px border border-border bg-border sm:grid-cols-4">
            <Cell label={t("apps.field.state")}>{running ? t("apps.overview.listening") : t("apps.overview.noBackend")}</Cell>
            <Cell label={t("apps.field.port")}>{meta.port || "-"}</Cell>
            <Cell label={t("apps.field.host")}>{domains[0] || "-"}</Cell>
            <Cell label={t("apps.field.runtime")}>{t("apps.overview.externalRuntime")}</Cell>
          </div>
          <div className="mt-3 border border-border bg-card/35 px-3 py-2">
            <p className="micro">{t("apps.overview.suggestedCommand")}</p>
            <div className="mt-1 flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate font-mono text-[11px]">{cmd}</code>
              <CopyButton text={cmd} />
            </div>
            <p className="mt-1 font-mono text-[9px] text-muted-foreground">
              {t("apps.overview.externalRunHint", { dir: meta.dir || t("apps.overview.repoFolder") })}
            </p>
          </div>
        </Section>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div
        className={
          domains[0]
            ? "grid grid-cols-1 items-start gap-4 xl:grid-cols-[minmax(320px,auto)_minmax(0,1fr)]"
            : "grid grid-cols-1 gap-4"
        }
      >
        {domains[0] && <AppPreview domain={domains[0]} running={running} />}

        <Section
          title={t("apps.overview.productionTitle")}
          className="h-full"
          description={prod ? t("apps.overview.rev", { rev: prod.rev.slice(0, 8), duration: dur(prod.started_at, prod.finished_at) }) : t("apps.overview.noSuccess")}
          accent={!prod}
        >
        {!prod && (
          <div className="border border-border bg-card/30 px-4 py-6 text-center">
            <p className="micro text-muted-foreground/60">{t("apps.overview.noRecord")}</p>
            <p className="mt-2 text-[12px]">{t("apps.overview.noDeploy")}</p>
            <p className="mt-1 font-mono text-[10px] text-muted-foreground">{t("apps.overview.publishHint")}</p>
          </div>
        )}
        {prod && (
          <>
            <div className="grid grid-cols-2 gap-px border border-border bg-border sm:grid-cols-4">
              <Cell label={t("apps.field.revision")}>
                <span className="flex items-center gap-1">
                  {prod.rev.slice(0, 8)}
                  <CopyButton text={prod.rev} />
                </span>
              </Cell>
              <Cell label={t("apps.field.duration")}>{dur(prod.started_at, prod.finished_at)}</Cell>
              <Cell label={t("apps.field.triggeredBy")}>{prod.triggered_by || "—"}</Cell>
              <Cell label={t("apps.field.time")}>
                <Time iso={prod.finished_at} />
              </Cell>
            </div>
            <div className="mt-3 border border-border">
              <div className="flex items-center gap-2 border-b border-border bg-secondary/20 px-3 py-1.5">
                <span className="micro">{t("apps.field.destinations")}</span>
                <span className="tnum ml-auto font-mono text-[9px] text-muted-foreground">{domains.length}</span>
              </div>
              {domains.length === 0 && (
                <p className="px-3 py-2 font-mono text-[10px] text-muted-foreground">
                  {t("apps.overview.noDomains")}
                </p>
              )}
              {domains.map((h) => (
                <a
                  key={h}
                  href={publicUrl(h, status)}
                  target="_blank"
                  rel="noreferrer"
                  className="group flex items-center gap-2 border-b border-border/60 px-3 py-1.5 last:border-0 hover:bg-accent/40"
                >
                  <span aria-hidden className="h-1 w-1 bg-ok" />
                  <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{publicHost(h, status)}</span>
                  <ExternalLink className="h-3 w-3 text-muted-foreground/50 group-hover:text-foreground" />
                </a>
              ))}
            </div>
          </>
        )}
        </Section>
      </div>

      <div className="grid grid-cols-1 items-start gap-4 md:grid-cols-2">
        <Section title={t("apps.overview.checklistTitle")} description={t("apps.overview.checklistDesc")}>
          <div className="divide-y divide-border/60">
            <Check ok={!!meta?.deploy?.repo} label={t("apps.overview.checkRepo")} hint={t("apps.overview.checkRepoHint")} />
            <Check ok={domains.length > 0} label={t("apps.overview.checkDomain")} hint={t("apps.overview.checkDomainHint")} />
            <Check ok={!!prod} label={t("apps.overview.checkDeploy")} hint={t("apps.overview.checkDeployHint")} />
            <Check ok={envCount > 0} label={t("apps.overview.checkEnv")} hint={t("nav.environment")} />
            <Check ok={running} label={t("apps.overview.checkRunning")} hint="supervisor" />
          </div>
        </Section>

        <Section title={t("apps.overview.trafficTitle")} description={domains[0] ?? t("apps.tabs.noDomain")}>
          {!stats && (
            <p className="border border-border bg-card/30 px-3 py-4 font-mono text-[10px] text-muted-foreground">
              {t("apps.overview.noData", {
                scope: domains[0] ? t("apps.overview.noDataFor", { domain: domains[0] }) : "",
              })}
            </p>
          )}
          {stats && (
            <div className="grid grid-cols-2 gap-px border border-border bg-border">
              <Stat value={`${stats.total}`} label={t("apps.overview.requests")} sub={t("apps.overview.window7d")} />
              <Stat value={`${(stats.bytes / 1048576).toFixed(1)}M`} label={t("apps.overview.traffic")} sub="access.log" />
              <Stat value={stats.p95 || "—"} label="p95" sub={t("apps.overview.latency")} />
              <Stat
                value={`${stats.by_class["5xx"] ?? 0}`}
                label="5xx"
                sub={t("apps.overview.serverErrors")}
                tone={(stats.by_class["5xx"] ?? 0) > 0 ? "warn" : undefined}
              />
            </div>
          )}
        </Section>
      </div>

      <div className="grid grid-cols-1 items-start gap-4 md:grid-cols-2">
        <Section
          title={t("apps.overview.recentDeploys")}
          description={t("apps.overview.recentDeploysDesc", { n: Math.min(appDeps.length, 5) })}
        >
          <div className="divide-y divide-border/60">
            {appDeps.slice(0, 5).map((d) => (
              <button
                key={d.id}
                type="button"
                onClick={() => setSel(d)}
                className="flex w-full items-center gap-2 px-3 py-2 text-left transition-colors hover:bg-accent/40"
              >
                <Dot status={d.status} />
                <span className="min-w-0 flex-1">
                  <span className="block font-mono text-[11px]">
                    #{d.id} · {d.rev.slice(0, 8) || "—"}
                  </span>
                  <span className="block font-mono text-[9px] text-muted-foreground">
                    {deployStateLabel(d.status)} · {rel(d.started_at)}
                  </span>
                </span>
                <span className="tnum shrink-0 font-mono text-[10px] text-muted-foreground">
                  {dur(d.started_at, d.finished_at)}
                </span>
                <ArrowRight className="h-3 w-3 shrink-0 text-muted-foreground/40" />
              </button>
            ))}
            {appDeps.length === 0 && (
              <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">{t("apps.overview.noDeploys")}</p>
            )}
          </div>
          <Link
            to={`/app/${name}/deploys`}
            className="flex items-center gap-1 border-t border-border px-3 py-2 font-mono text-[10px] text-muted-foreground transition-colors hover:text-foreground"
          >
            {t("apps.overview.viewDeploys")} <ArrowRight className="h-3 w-3" />
          </Link>
        </Section>

        <Section title={t("apps.overview.recentActivity")} description={t("apps.overview.recentActivityDesc")}>
          <div className="divide-y divide-border/60">
            {appEvents.slice(0, 6).map((e) => (
              <div
                key={e.id}
                className="grid grid-cols-[4rem_4.5rem_minmax(0,1fr)] items-baseline gap-2 px-3 py-1.5"
              >
                <span className="tnum font-mono text-[10px] text-muted-foreground/60">{rel(e.ts)}</span>
                <span className="truncate font-mono text-[10px] tracking-[0.06em] text-muted-foreground uppercase">
                  {eventKindLabel(e.kind)}
                </span>
                <span className="min-w-0 truncate font-mono text-[11px] text-foreground/70">{e.msg}</span>
              </div>
            ))}
            {appEvents.length === 0 && (
              <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">{t("apps.overview.noActivity")}</p>
            )}
          </div>
          <Link
            to={`/app/${name}/logs`}
            className="flex items-center gap-1 border-t border-border px-3 py-2 font-mono text-[10px] text-muted-foreground transition-colors hover:text-foreground"
          >
            {t("apps.overview.viewLogs")} <ArrowRight className="h-3 w-3" />
          </Link>
        </Section>
      </div>
    </div>
  );
}
