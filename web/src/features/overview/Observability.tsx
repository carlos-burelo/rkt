import { useEffect, useState } from "react";
import { Link } from "react-router";
import { fetchObs, Obs } from "./api";
import { Bars } from "../../shared/components/feedback";
import { PageHeader, Stat, StatGrid, Section, Segmented, EmptyState } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";

export function Observability() {
  const { t, tn } = useI18n();
  const [hours, setHours] = useState(6);
  const [data, setData] = useState<Obs | null>(null);

  useEffect(() => {
    setData(null);
    fetchObs(hours)
      .then(setData)
      .catch(() => setData(null));
  }, [hours]);

  const label = hours === 168 ? "7d" : `${hours}h`;
  const reqTotal = data ? data.buckets.reduce((a, b) => a + b.req, 0) : 0;
  const errTotal = data ? data.buckets.reduce((a, b) => a + b.err, 0) : 0;

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("nav.observability")}
        description={t("overview.observability.description")}
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{t("overview.window", { label })}</span>}
        actions={
          <Segmented
            value={String(hours)}
            onChange={(v) => setHours(Number(v))}
            label={t("overview.observability.windowAria")}
            items={[6, 24, 168].map((h) => ({ value: String(h), label: h === 168 ? "7d" : `${h}h` }))}
          />
        }
      />
      {!data && <p className="font-mono text-[10px] text-muted-foreground">{t("overview.loadingSeries")}</p>}
      {data && reqTotal === 0 && (data.by_app?.length ?? 0) === 0 && (
        <EmptyState title={t("overview.observability.emptyTitle")} hint={t("overview.observability.emptyHint")} />
      )}
      {data && (reqTotal > 0 || (data.by_app?.length ?? 0) > 0) && (
        <>
          <StatGrid className="md:grid-cols-4">
            <Stat value={`${reqTotal}`} label={t("overview.requests")} sub={t("overview.window", { label })} />
            <Stat value={data.p95 || "—"} label="p95" sub={`p50 ${data.p50 || "—"} · p99 ${data.p99 || "—"}`} />
            <Stat
              value={`${data.restarts}`}
              label={t("overview.observability.restarts")}
              sub={t("overview.observability.restartsSub")}
              tone={data.restarts > 0 ? "warn" : undefined}
            />
            <Stat
              value={data.down.length === 0 ? "0" : `${data.down.length}`}
              label={t("overview.observability.down")}
              sub={data.down.length === 0 ? t("overview.observability.downNone") : data.down.join(", ")}
              tone={data.down.length === 0 ? "ok" : "bad"}
            />
          </StatGrid>
          <Section
            title={t("overview.observability.reqTitle")}
            description={t("overview.observability.reqDesc", { n: data.buckets.length })}
            actions={<span className="tnum font-mono text-[9px] text-muted-foreground">{t("overview.total", { n: reqTotal })}</span>}
          >
            <Bars data={data.buckets.map((b) => b.req)} labels={data.buckets.map((b) => (b.ts.length > 5 ? b.ts.slice(11, 16) : b.ts))} height={104} showValues />
          </Section>
          <Section
            title={t("overview.observability.errTitle")}
            description={errTotal === 0 ? t("overview.observability.errNone") : t("overview.observability.errAction")}
            actions={<span className={`tnum font-mono text-[9px] ${errTotal > 0 ? "text-bad" : "text-muted-foreground"}`}>{tn("overview.errors", errTotal)}</span>}
          >
            <Bars data={data.buckets.map((b) => b.err)} height={64} tone={(v) => (v > 0 ? "bg-bad/80" : "bg-muted/70")} />
          </Section>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <Section title={t("overview.observability.classes")} description={t("overview.observability.classesDesc")}>
              <div className="grid grid-cols-2 gap-px border border-border bg-border sm:grid-cols-4">
                {["2xx", "3xx", "4xx", "5xx"].map((k) => (
                  <div key={k} className="bg-card/45 px-3 py-2">
                    <p className="micro">{k}</p>
                    <p className="tnum mt-1 font-mono text-[14px]">{data.classes?.[k] ?? 0}</p>
                  </div>
                ))}
              </div>
            </Section>
            <Section
              title={t("overview.observability.byApp")}
              description={t("overview.observability.byAppDesc")}
              actions={<span className="tnum font-mono text-[9px] text-muted-foreground">{data.by_app?.length ?? 0}</span>}
            >
              <div className="divide-y divide-border/60">
                {(data.by_app ?? []).map((a) => (
                  <Link key={a.app} to={`/app/${a.app}/logs`} className="flex items-center gap-2 px-3 py-1.5 hover:bg-accent/40">
                    <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{a.app}</span>
                    <span className="tnum font-mono text-[10px] text-muted-foreground">{a.req}</span>
                    <span className={`tnum w-10 text-right font-mono text-[10px] ${a.err > 0 ? "text-bad" : "text-muted-foreground"}`}>
                      {a.err}
                    </span>
                    <span className="tnum w-16 text-right font-mono text-[10px] text-muted-foreground">
                      {a.p95 || "—"}
                    </span>
                  </Link>
                ))}
                {(data.by_app ?? []).length === 0 && (
                  <p className="px-3 py-3 font-mono text-[10px] text-muted-foreground">{t("common.empty")}</p>
                )}
              </div>
            </Section>
          </div>
        </>
      )}
    </div>
  );
}
