import { useEffect, useState } from "react";
import { Analytics, fetchAnalytics } from "./api";
import { Bars } from "../../shared/components/feedback";
import { PageHeader, Stat, StatGrid, Section, Segmented } from "../../shared/components/patterns";
import { Err } from "../../shared/components/ui";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";

const RANGES = [1, 7, 30] as const;

export function AnalyticsPage() {
  const [host, setHost] = useState("");
  const [hosts, setHosts] = useState<string[]>([]);
  const [days, setDays] = useState<number>(7);
  const [data, setData] = useState<Analytics | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    fetchAnalytics("", days)
      .then((g) => setHosts(g.by_host.map((h) => h.k)))
      .catch(() => {});
  }, [days]);

  useEffect(() => {
    setData(null);
    setErr("");
    fetchAnalytics(host, days)
      .then(setData)
      .catch((e) => setErr(e instanceof Error ? e.message : "error"));
  }, [host, days]);

  const errCount = data ? (data.by_class["5xx"] ?? 0) : 0;
  const errPct = data && data.total > 0 ? `${((errCount / data.total) * 100).toFixed(1)}%` : "—";
  const dayKeys = data ? Object.keys(data.by_day).sort() : [];
  const series = dayKeys.map((k) => data?.by_day[k] ?? 0);
  const windowLabel = days === 1 ? "24h" : `${days}d`;

  return (
    <div className="space-y-4">
      <PageHeader
        title="Analítica"
        description="Tráfico real desde access.log. Sin visitors ni bounce inventados; lo que no se mide, no se pinta."
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">ventana {windowLabel}</span>}
      />
      <div className="flex flex-wrap items-center gap-2 border border-border bg-card/35 p-2">
        <Select value={host || "__all"} onValueChange={(v: string | null) => setHost(v === "__all" || v == null ? "" : v)}>
          <SelectTrigger className="tnum h-7 w-56 font-mono text-[10px]">
            <span className="truncate">{host || "todos los hosts"}</span>
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
        <Segmented
          value={String(days)}
          onChange={(v) => setDays(Number(v))}
          label="rango de la ventana"
          items={RANGES.map((d) => ({ value: String(d), label: d === 1 ? "24h" : `${d}d` }))}
        />
        <span className="flex-1" />
        <span className="folio">access.log local</span>
      </div>
      {err !== "" && <Err msg={err} />}
      {!data && err === "" && <p className="font-mono text-[10px] text-muted-foreground">cargando serie…</p>}
      {data && (
        <>
          <StatGrid>
            <Stat value={`${data.total}`} label="peticiones" sub={`ventana ${windowLabel}`} />
            <Stat value={`${(data.bytes / 1048576).toFixed(1)}M`} label="tráfico" sub="access.log" />
            <Stat value={errPct} label="tasa de error" sub={`${errCount} 5xx`} tone={errCount > 0 ? "warn" : undefined} />
            <Stat value={data.p95 || "—"} label="p95" sub={`p50 ${data.p50 || "—"}`} />
          </StatGrid>
          <Section
            title="Peticiones por día"
            description={`${dayKeys[0] ?? "sin fecha"} → ${dayKeys[dayKeys.length - 1] ?? "sin fecha"} · access.log local`}
            actions={<span className="tnum font-mono text-[9px] text-muted-foreground">{dayKeys.length} días</span>}
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
                ["Top rutas", data.by_path],
                ["Top hosts", data.by_host],
                ["Top referers", data.by_ref],
              ] as const
            ).map(([title, rows]) => (
              <Section
                key={title}
                title={title}
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
                  {rows.length === 0 && <p className="py-2 font-mono text-[10px] text-muted-foreground">sin datos</p>}
                </div>
              </Section>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
