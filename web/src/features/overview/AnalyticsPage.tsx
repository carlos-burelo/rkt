import { useEffect, useState } from "react";
import { Analytics, fetchAnalytics } from "./api";
import { Bars } from "../../shared/components/feedback";
import { PageHeader, Stat, Section } from "../../shared/components/patterns";
import { Badge } from "@/components/ui/badge";
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
        <div className="flex gap-1.5" role="group" aria-label="rango">
          {RANGES.map((d) => (
            <Badge
              key={d}
              variant={days === d ? "default" : "outline"}
              className="tnum cursor-pointer font-mono text-[11px] font-normal"
              onClick={() => setDays(d)}
            >
              {d === 1 ? "24h" : `${d}d`}
            </Badge>
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
          <Section title="Requests por día" description={`${dayKeys[0] ?? ""} → ${dayKeys[dayKeys.length - 1] ?? ""} · access.log local`}>
            <Bars data={series} height={96} />
          </Section>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            {(
              [
                ["Top rutas", data.by_path],
                ["Top hosts", data.by_host],
                ["Top referers", data.by_ref],
              ] as const
            ).map(([title, rows]) => (
              <div key={title} className="overflow-hidden rounded-lg border">
                <div className="tnum border-b bg-muted/40 px-4 py-1.5 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
                  {title} · {rows.length}
                </div>
                {rows.map((r, i) => (
                  <div key={r.k} className="flex items-center gap-3 border-b bg-card/40 px-4 py-2 last:border-0">
                    <span className="tnum w-6 shrink-0 font-mono text-[11px] text-muted-foreground">{String(i + 1).padStart(2, "0")}</span>
                    <span className="min-w-0 flex-1 truncate font-mono text-xs">{r.k}</span>
                    <span className="tnum font-mono text-[11px] text-muted-foreground">{r.v}</span>
                  </div>
                ))}
                {rows.length === 0 && <p className="bg-card/40 px-4 py-3 text-xs text-muted-foreground">sin datos</p>}
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
