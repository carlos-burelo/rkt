import { useEffect, useState } from "react";
import { fetchObs, Obs } from "./api";
import { Bars } from "../../shared/components/feedback";
import { PageHeader, Stat, Section } from "../../shared/components/patterns";
import { Badge } from "@/components/ui/badge";

export function Observability() {
  const [hours, setHours] = useState(6);
  const [data, setData] = useState<Obs | null>(null);

  useEffect(() => {
    setData(null);
    fetchObs(hours)
      .then(setData)
      .catch(() => setData(null));
  }, [hours]);

  return (
    <div className="space-y-4">
      <PageHeader title="Observability" description="Serie por hora, restarts y apps caídas. Latencia real en Analytics." />
      <div className="flex gap-1.5" role="group" aria-label="ventana">
        {[6, 24, 168].map((h) => (
          <Badge
            key={h}
            variant={hours === h ? "default" : "outline"}
            className="tnum cursor-pointer font-mono text-[11px] font-normal"
            onClick={() => setHours(h)}
          >
            {h === 168 ? "7d" : `${h}h`}
          </Badge>
        ))}
      </div>
      {!data && <p className="text-sm text-muted-foreground">cargando…</p>}
      {data && (
        <>
          <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
            <Stat value={`${data.buckets.reduce((a, b) => a + b.req, 0)}`} label="requests" sub={`ventana ${hours}h`} />
            <Stat value={`${data.restarts}`} label="restarts" sub="events died/restarted" tone={data.restarts > 0 ? "warn" : undefined} />
            <Stat
              value={data.down.length === 0 ? "0" : `${data.down.length}`}
              label="apps caídas"
              sub={data.down.length === 0 ? "flota en ruta" : data.down.join(", ")}
              tone={data.down.length === 0 ? "ok" : "bad"}
            />
          </div>
          <Section title="Requests por hora" description={`ventana ${hours}h · serie local`}>
            <Bars data={data.buckets.map((b) => b.req)} height={96} />
          </Section>
          <Section title="Errores 5xx por hora" description={data.buckets.reduce((a, b) => a + b.err, 0) === 0 ? "sin 5xx en ventana" : "requieren mirada en Logs"}>
            <Bars data={data.buckets.map((b) => b.err)} height={64} tone={(v) => (v > 0 ? "bg-red-400/80" : "bg-muted")} />
          </Section>
        </>
      )}
    </div>
  );
}
