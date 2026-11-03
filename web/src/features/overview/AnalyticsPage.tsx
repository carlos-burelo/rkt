import { useEffect, useState } from "react";
import { Analytics, fetchAnalytics } from "./api";
import { Bars } from "../../shared/components/feedback";
import { PageHeader, Stat } from "../../shared/components/patterns";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

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
      <PageHeader title="Analytics" description="Tráfico real desde access.log. Sin visitors ni bounce inventados." />
      <div className="flex flex-wrap items-center gap-2">
        <Select value={host || "__all"} onValueChange={(v: string | null) => setHost(v === "__all" || v == null ? "" : v)}>
          <SelectTrigger className="tnum w-56 font-mono text-xs">
            <SelectValue placeholder="todos los hosts" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all">todos los hosts</SelectItem>
            {hosts.map((h) => (
              <SelectItem key={h} value={h}>
                {h}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="flex rounded-md border p-0.5" role="group" aria-label="rango">
          {RANGES.map((d) => (
            <button
              key={d}
              onClick={() => setDays(d)}
              className={`tnum rounded px-2.5 py-1 font-mono text-xs ${days === d ? "bg-accent text-foreground" : "text-muted-foreground hover:text-foreground"}`}
            >
              {d === 1 ? "24h" : `${d}d`}
            </button>
          ))}
        </div>
      </div>
      {err !== "" && <p className="text-xs text-destructive">{err}</p>}
      {!data && <p className="text-sm text-muted-foreground">cargando…</p>}
      {data && (
        <>
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
            <Stat value={`${data.total}`} label="requests" sub={`ventana ${days === 1 ? "24h" : `${days}d`}`} />
            <Stat value={`${(data.bytes / 1048576).toFixed(1)}M`} label="bytes" sub="access.log" />
            <Stat value={errPct} label="error rate" sub={`${errCount} 5xx`} tone={errCount > 0 ? "warn" : undefined} />
            <Stat value={data.p95 || "—"} label="p95" sub="latencia" />
          </div>
          <div className="rounded-lg border bg-card p-4">
            <p className="mb-2 text-sm font-medium">Requests por día</p>
            <Bars data={series} height={96} />
            <div className="tnum mt-1 flex justify-between font-mono text-[10px] text-muted-foreground">
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
              <div key={title} className="rounded-lg border bg-card p-4">
                <p className="mb-2 text-sm font-medium">{title}</p>
                {rows.map((r) => (
                  <p key={r.k} className="tnum flex justify-between gap-2 truncate py-0.5 font-mono text-xs">
                    <span className="truncate">{r.k}</span>
                    <span className="text-muted-foreground">{r.v}</span>
                  </p>
                ))}
                {rows.length === 0 && <p className="text-xs text-muted-foreground">sin datos</p>}
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
