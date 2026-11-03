import { useEffect, useState } from "react";
import { fetchObs, Obs } from "./api";
import { Bars } from "../../shared/components/Bars";

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
      <div className="flex rounded-md border border-zinc-800 p-0.5 w-fit">
        {[6, 24, 168].map((h) => (
          <button
            key={h}
            onClick={() => setHours(h)}
            className={`rounded px-2.5 py-1 font-mono text-xs ${hours === h ? "bg-zinc-700 text-white" : "text-zinc-400"}`}
          >
            {h === 168 ? "7d" : `${h}h`}
          </button>
        ))}
      </div>
      {!data && <p className="text-sm text-zinc-500">cargando…</p>}
      {data && (
        <>
          <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
            <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
              <p className="font-mono text-xl">{data.buckets.reduce((a, b) => a + b.req, 0)}</p>
              <p className="mt-1 text-xs text-zinc-500">requests</p>
            </div>
            <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
              <p className="font-mono text-xl">{data.restarts}</p>
              <p className="mt-1 text-xs text-zinc-500">restarts</p>
            </div>
            <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
              <p className="font-mono text-xl">{data.down.length === 0 ? "0" : data.down.join(", ")}</p>
              <p className="mt-1 text-xs text-zinc-500">apps caídas</p>
            </div>
          </div>
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
            <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">requests por hora</p>
            <Bars data={data.buckets.map((b) => b.req)} height={96} />
          </div>
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
            <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">errores 5xx por hora</p>
            <Bars data={data.buckets.map((b) => b.err)} height={64} />
          </div>
        </>
      )}
    </div>
  );
}
