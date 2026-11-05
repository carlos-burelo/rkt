import { useEffect, useState } from "react";
import { fetchObs, Obs } from "./api";
import { Bars } from "../../shared/components/feedback";
import { PageHeader, Stat, StatGrid, Section, Segmented } from "../../shared/components/patterns";
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
      {data && (
        <>
          <StatGrid className="md:grid-cols-3">
            <Stat value={`${reqTotal}`} label={t("overview.requests")} sub={t("overview.window", { label })} />
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
            <Bars data={data.buckets.map((b) => b.req)} height={104} />
          </Section>
          <Section
            title={t("overview.observability.errTitle")}
            description={errTotal === 0 ? t("overview.observability.errNone") : t("overview.observability.errAction")}
            actions={<span className={`tnum font-mono text-[9px] ${errTotal > 0 ? "text-bad" : "text-muted-foreground"}`}>{tn("overview.errors", errTotal)}</span>}
          >
            <Bars data={data.buckets.map((b) => b.err)} height={64} tone={(v) => (v > 0 ? "bg-bad/80" : "bg-muted/70")} />
          </Section>
        </>
      )}
    </div>
  );
}
