import { useEffect, useState } from "react";
import { fetchObs, Obs } from "./api";
import { Bars } from "../../shared/components/feedback";
import { PageHeader, Stat, StatGrid, Section, Segmented } from "../../shared/components/patterns";

export function Observability() {
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
        title="Observability"
        description="Serie por hora, restarts y apps caídas. Latencia real en Analytics; aquí solo lo que el supervisor y access.log pueden probar."
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">ventana {label}</span>}
        actions={
          <Segmented
            value={String(hours)}
            onChange={(v) => setHours(Number(v))}
            label="ventana de observación"
            items={[6, 24, 168].map((h) => ({ value: String(h), label: h === 168 ? "7d" : `${h}h` }))}
          />
        }
      />
      {!data && <p className="font-mono text-[10px] text-muted-foreground">cargando serie…</p>}
      {data && (
        <>
          <StatGrid className="md:grid-cols-3">
            <Stat value={`${reqTotal}`} label="requests" sub={`ventana ${label}`} />
            <Stat value={`${data.restarts}`} label="restarts" sub="events died/restarted" tone={data.restarts > 0 ? "warn" : undefined} />
            <Stat
              value={data.down.length === 0 ? "0" : `${data.down.length}`}
              label="apps caídas"
              sub={data.down.length === 0 ? "flota en ruta" : data.down.join(", ")}
              tone={data.down.length === 0 ? "ok" : "bad"}
            />
          </StatGrid>
          <Section
            title="Requests por hora"
            description={`serie local · ${data.buckets.length} buckets`}
            actions={<span className="tnum font-mono text-[9px] text-muted-foreground">{reqTotal} total</span>}
          >
            <Bars data={data.buckets.map((b) => b.req)} height={104} />
          </Section>
          <Section
            title="Errores 5xx por hora"
            description={errTotal === 0 ? "sin 5xx en la ventana" : "requieren mirada en Logs"}
            actions={<span className={`tnum font-mono text-[9px] ${errTotal > 0 ? "text-bad" : "text-muted-foreground"}`}>{errTotal} errores</span>}
          >
            <Bars data={data.buckets.map((b) => b.err)} height={64} tone={(v) => (v > 0 ? "bg-bad/80" : "bg-muted/70")} />
          </Section>
        </>
      )}
    </div>
  );
}
