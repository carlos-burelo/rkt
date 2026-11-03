import { useEffect, useState } from "react";
import { Analytics, fetchAnalytics } from "./api";
import { Bars } from "../../shared/components/Bars";

const RANGES = [1, 7, 30];

export function AnalyticsPage() {
  const [host, setHost] = useState("");
  const [hosts, setHosts] = useState<string[]>([]);
  const [days, setDays] = useState(7);
  const [data, setData] = useState<Analytics | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    fetchAnalytics("", days)
      .then((g) => setHosts(g.by_host.map((h) => h.k)))
      .catch(() => {});
  }, [days]);

  useEffect(() => {
    setData(null);
    fetchAnalytics(host, days)
      .then(setData)
      .catch((e) => setErr(e instanceof Error ? e.message : "error"));
  }, [host, days]);

  const errCount = data ? (data.by_class["5xx"] ?? 0) : 0;
  const errPct = data && data.total > 0 ? `${((errCount / data.total) * 100).toFixed(1)}%` : "—";
  const dayKeys = data ? Object.keys(data.by_day).sort() : [];
  const series = dayKeys.map((k) => data?.by_day[k] ?? 0);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <select
          value={host}
          onChange={(e) => setHost(e.target.value)}
          className="rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none"
        >
          <option value="">todos los hosts</option>
          {hosts.map((h) => (
            <option key={h} value={h}>
              {h}
            </option>
          ))}
        </select>
        <div className="flex rounded-md border border-zinc-800 p-0.5">
          {RANGES.map((d) => (
            <button
              key={d}
              onClick={() => setDays(d)}
              className={`rounded px-2.5 py-1 font-mono text-xs ${days === d ? "bg-zinc-700 text-white" : "text-zinc-400"}`}
            >
              {d === 1 ? "24h" : `${d}d`}
            </button>
          ))}
        </div>
      </div>
      {err !== "" && <p className="text-xs text-red-400">{err}</p>}
      {!data && <p className="text-sm text-zinc-500">cargando…</p>}
      {data && (
        <>
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
            {(
              [
                [`${data.total}`, "requests"],
                [`${(data.bytes / 1048576).toFixed(1)}M`, "bytes"],
                [errPct, "error rate"],
                [data.p95 || "—", "p95"],
              ] as const
            ).map(([v, label]) => (
              <div key={label} className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
                <p className="font-mono text-xl">{v}</p>
                <p className="mt-1 text-xs text-zinc-500">{label}</p>
              </div>
            ))}
          </div>
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
            <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">requests por día</p>
            <Bars data={series} height={96} />
            <div className="mt-1 flex justify-between font-mono text-[10px] text-zinc-600">
              <span>{dayKeys[0] ?? ""}</span>
              <span>{dayKeys[dayKeys.length - 1] ?? ""}</span>
            </div>
          </div>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            {(
              [
                ["Top rutas", data.by_path],
                ["Top hosts", data.by_host],
                ["Top referers", data.by_ref],
              ] as const
            ).map(([title, rows]) => (
              <div key={title} className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
                <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">{title}</p>
                {rows.map((r) => (
                  <p key={r.k} className="flex justify-between gap-2 truncate py-0.5 font-mono text-xs">
                    <span className="truncate text-zinc-300">{r.k}</span>
                    <span className="text-zinc-500">{r.v}</span>
                  </p>
                ))}
                {rows.length === 0 && <p className="text-xs text-zinc-600">sin datos</p>}
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
