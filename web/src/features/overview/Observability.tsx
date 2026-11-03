import { useEffect, useState } from "react";
import { fetchObs, Obs } from "./api";
import { Bars } from "../../shared/components/feedback";
import { PageHeader, Stat } from "../../shared/components/patterns";

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
      <div className="flex w-fit rounded-md border p-0.5" role="group" aria-label="ventana">
        {[6, 24, 168].map((h) => (
          <button
            key={h}
            onClick={() => setHours(h)}
            className={`tnum rounded px-2.5 py-1 font-mono text-xs ${hours === h ? "bg-accent text-foreground" : "text-muted-foreground hover:text-foreground"}`}
          >
            {h === 168 ? "7d" : `${h}h`}
          </button>
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
          <div className="rounded-lg border bg-card p-4">
            <p className="mb-2 text-sm font-medium">Requests por hora</p>
            <Bars data={data.buckets.map((b) => b.req)} height={96} />
          </div>
          <div className="rounded-lg border bg-card p-4">
            <p className="mb-2 text-sm font-medium">Errores 5xx por hora</p>
            <Bars data={data.buckets.map((b) => b.err)} height={64} />
          </div>
        </>
      )}
    </div>
  );
}
