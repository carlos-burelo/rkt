import { useEffect } from "react";
import { Bars } from "../../shared/components/feedback";
import { PageHeader, Stat, StatGrid, Section, Segmented } from "../../shared/components/patterns";
import { Err } from "../../shared/components/ui";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { useI18n } from "../../shared/i18n";
import { useAnalyticsPage } from "./analytics.store";

const RANGES = [1, 7, 30] as const;

export function AnalyticsPage() {
  const { t, tn } = useI18n();
  const host = useAnalyticsPage((s) => s.host);
  const hosts = useAnalyticsPage((s) => s.hosts);
  const days = useAnalyticsPage((s) => s.days);
  const data = useAnalyticsPage((s) => s.data);
  const err = useAnalyticsPage((s) => s.err);

  useEffect(() => {
    useAnalyticsPage.getState().load();
  }, []);

  const errCount = data ? (data.by_class["5xx"] ?? 0) : 0;
  const errPct = data && data.total > 0 ? `${((errCount / data.total) * 100).toFixed(1)}%` : "—";
  const dayKeys = data ? Object.keys(data.by_day).sort() : [];
  const series = dayKeys.map((k) => data?.by_day[k] ?? 0);
  const windowLabel = days === 1 ? "24h" : `${days}d`;

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("nav.analytics")}
        description={t("overview.analytics.description")}
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{t("overview.window", { label: windowLabel })}</span>}
      />
      <div className="flex flex-wrap items-center gap-2 border border-border bg-card/35 p-2">
        <Select value={host || "__all"} onValueChange={(v: string | null) => useAnalyticsPage.getState().setHost(v === "__all" || v == null ? "" : v)}>
          <SelectTrigger className="tnum h-7 w-56 font-mono text-[10px]">
            <span className="truncate">{host || t("overview.analytics.allHosts")}</span>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all">{t("overview.analytics.allHosts")}</SelectItem>
            {hosts.map((h) => (
              <SelectItem key={h} value={h}>
                {h}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Segmented
          value={String(days)}
          onChange={(v) => useAnalyticsPage.getState().setDays(Number(v))}
          label={t("overview.analytics.rangeAria")}
          items={RANGES.map((d) => ({ value: String(d), label: d === 1 ? "24h" : `${d}d` }))}
        />
        <span className="flex-1" />
        <span className="folio">access.log local</span>
      </div>
      {err !== "" && <Err msg={err} />}
      {!data && err === "" && <p className="font-mono text-[10px] text-muted-foreground">{t("overview.loadingSeries")}</p>}
      {data && (
        <>
          <StatGrid>
            <Stat value={`${data.total}`} label={t("overview.requests")} sub={t("overview.window", { label: windowLabel })} />
            <Stat value={`${(data.bytes / 1048576).toFixed(1)}M`} label={t("overview.analytics.traffic")} sub="access.log" />
            <Stat value={errPct} label={t("overview.errorRate")} sub={`${errCount} 5xx`} tone={errCount > 0 ? "warn" : undefined} />
            <Stat value={data.p95 || "—"} label="p95" sub={`p50 ${data.p50 || "—"}`} />
          </StatGrid>
          <Section
            title={t("overview.analytics.perDay")}
            description={t("overview.analytics.perDayDesc", {
              from: dayKeys[0] ?? t("overview.analytics.noDate"),
              to: dayKeys[dayKeys.length - 1] ?? t("overview.analytics.noDate"),
            })}
            actions={<span className="tnum font-mono text-[9px] text-muted-foreground">{tn("overview.days", dayKeys.length)}</span>}
          >
            <Bars
              data={series}
              labels={dayKeys.map((k) => k.slice(5))}
              height={104}
              showValues
            />
          </Section>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            {(
              [
                ["overview.analytics.topPaths", data.by_path],
                ["overview.analytics.topHosts", data.by_host],
                ["overview.analytics.topRefs", data.by_ref],
              ] as const
            ).map(([title, rows]) => (
              <Section
                key={title}
                title={t(title)}
                actions={<span className="tnum font-mono text-[9px] text-muted-foreground">{rows.length}</span>}
              >
                <div className="divide-y divide-border/60">
                  {[...rows].sort((a, b) => b.v - a.v).map((r, i) => (
                    <div key={r.k} className="flex items-center gap-2 py-1.5">
                      <span className="folio w-5 shrink-0">{String(i + 1).padStart(2, "0")}</span>
                      <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{r.k}</span>
                      <span className="tnum font-mono text-[10px] text-muted-foreground">{r.v}</span>
                    </div>
                  ))}
                  {rows.length === 0 && <p className="py-2 font-mono text-[10px] text-muted-foreground">{t("common.empty")}</p>}
                </div>
              </Section>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
