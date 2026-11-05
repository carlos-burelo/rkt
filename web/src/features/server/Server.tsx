import { useEffect, useState } from "react";
import { Activity, HardDrive, ShieldCheck, Thermometer } from "lucide-react";
import { fetchServer, fetchServerHistory, fetchServerProcesses, ServerPoint, ServerProcess, ServerSnapshot } from "./api";
import { Bars } from "../../shared/components/feedback";
import { EmptyState, Meter, PageHeader, Section, Stat, StatGrid } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";

function bytes(n: number): string {
  if (!Number.isFinite(n)) return "—";
  if (n >= 1 << 30) return `${(n / (1 << 30)).toFixed(1)} GB`;
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(0)} KB`;
  return `${Math.round(n)} B`;
}

function rate(n: number): string {
  return `${bytes(n)}/s`;
}

function uptime(s: number, t: (k: string, v?: Record<string, string | number>) => string): string {
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return t("server.uptime.days", { d, h });
  if (h > 0) return t("server.uptime.hours", { h, m });
  return t("server.uptime.minutes", { m });
}

function tone(frac: number | undefined) {
  if (frac === undefined) return "signal" as const;
  if (frac > 0.9) return "bad" as const;
  if (frac > 0.7) return "warn" as const;
  return "ok" as const;
}

export function Server() {
  const { t } = useI18n();
  const [snap, setSnap] = useState<ServerSnapshot | null>(null);
  const [history, setHistory] = useState<ServerPoint[]>([]);
  const [procs, setProcs] = useState<ServerProcess[]>([]);
  const [err, setErr] = useState("");

  useEffect(() => {
    let alive = true;
    const loadSnap = () => {
      fetchServer()
        .then((s) => {
          if (alive) {
            setSnap(s);
            setErr("");
          }
        })
        .catch((e) => alive && setErr(e instanceof Error ? e.message : t("common.error")));
      fetchServerHistory(6)
        .then((h) => alive && setHistory(h))
        .catch(() => {});
    };
    loadSnap();
    const snapTimer = setInterval(loadSnap, 5000);
    const procTimer = setInterval(() => {
      fetchServerProcesses(10)
        .then((p) => alive && setProcs(p))
        .catch(() => {});
    }, 15000);
    fetchServerProcesses(10)
      .then((p) => alive && setProcs(p))
      .catch(() => {});
    return () => {
      alive = false;
      clearInterval(snapTimer);
      clearInterval(procTimer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (!snap && err === "") {
    return <p className="font-mono text-[11px] text-muted-foreground">{t("server.loading")}</p>;
  }
  if (err !== "") {
    return <EmptyState title={t("server.unavailable")} hint={err} />;
  }
  if (!snap) return null;

  const diskMax = snap.disks.reduce((m, d) => Math.max(m, d.percent), 0);
  const tempMax = snap.temps.reduce((m, x) => Math.max(m, x.celsius), 0);
  const netRate = snap.network.rx_rate + snap.network.tx_rate;
  const labels = history.map((p, i) => (history.length > 8 && i % 2 === 1 ? "" : p.ts.slice(11, 16)));

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("nav.server")}
        description={t("server.description")}
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {snap.hostname} · {snap.os} · {uptime(snap.uptime_s, t)}
          </span>
        }
      />

      <StatGrid>
        <Stat value={`${snap.cpu.percent.toFixed(0)}%`} label={t("server.cpu")} sub={`${snap.cpu.cores} ${t("server.cores")}`} tone={snap.cpu.percent > 85 ? "warn" : "ok"} />
        <Stat value={`${snap.memory.percent.toFixed(0)}%`} label={t("server.memory")} sub={`${bytes(snap.memory.used)} / ${bytes(snap.memory.total)}`} tone={snap.memory.percent > 85 ? "warn" : "ok"} />
        <Stat value={`${diskMax.toFixed(0)}%`} label={t("server.disk")} sub={`${snap.disks.length} ${t("server.mounts")}`} tone={diskMax > 85 ? "warn" : "ok"} />
        <Stat value={rate(netRate)} label={t("server.network")} sub={`↓ ${rate(snap.network.rx_rate)} · ↑ ${rate(snap.network.tx_rate)}`} />
      </StatGrid>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Section title={t("server.cpuHistory")} description={t("server.cpuHistoryDesc")} actions={<span className="tnum font-mono text-[9px] text-muted-foreground">{history.length}</span>}>
          {history.length > 0 ? (
            <Bars data={history.map((p) => p.cpu)} labels={labels} height={96} showValues />
          ) : (
            <p className="font-mono text-[10px] text-muted-foreground">{t("server.noHistory")}</p>
          )}
        </Section>
        <Section title={t("server.memoryHistory")} description={t("server.memoryHistoryDesc")}>
          {history.length > 0 ? (
            <Bars data={history.map((p) => p.mem)} labels={labels} height={96} showValues />
          ) : (
            <p className="font-mono text-[10px] text-muted-foreground">{t("server.noHistory")}</p>
          )}
        </Section>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <Section title={t("server.memory")} description={t("server.memoryDesc")}>
          <div className="space-y-3">
            <Meter label={t("server.ram")} value={`${bytes(snap.memory.used)} / ${bytes(snap.memory.total)}`} frac={snap.memory.percent / 100} tone={tone(snap.memory.percent / 100)} />
            {snap.memory.swap_total > 0 && (
              <Meter
                label={t("server.swap")}
                value={`${bytes(snap.memory.swap_used)} / ${bytes(snap.memory.swap_total)}`}
                frac={snap.memory.swap_used / snap.memory.swap_total}
                tone={tone(snap.memory.swap_used / snap.memory.swap_total)}
              />
            )}
            <div className="grid grid-cols-2 gap-px border border-border bg-border">
              <div className="bg-card/45 px-3 py-2">
                <p className="micro">{t("server.available")}</p>
                <p className="tnum mt-1 font-mono text-[12px]">{bytes(snap.memory.available)}</p>
              </div>
              <div className="bg-card/45 px-3 py-2">
                <p className="micro">{t("server.cached")}</p>
                <p className="tnum mt-1 font-mono text-[12px]">{bytes(snap.memory.cached ?? 0)}</p>
              </div>
            </div>
          </div>
        </Section>

        <Section title={t("server.load")} description={snap.load.supported ? t("server.loadDesc") : t("server.loadUnsupported")}>
          <div className="grid grid-cols-3 gap-px border border-border bg-border">
            {[snap.load.l1, snap.load.l5, snap.load.l15].map((v, i) => (
              <div key={i} className="bg-card/45 px-3 py-2">
                <p className="micro">load {i === 0 ? "1" : i === 1 ? "5" : "15"}</p>
                <p className="tnum mt-1 font-mono text-[14px]">{snap.load.supported ? v.toFixed(2) : "—"}</p>
              </div>
            ))}
          </div>
          <div className="mt-3 grid grid-cols-2 gap-px border border-border bg-border">
            <div className="bg-card/45 px-3 py-2">
              <p className="micro">{t("server.uptime")}</p>
              <p className="mt-1 font-mono text-[12px]">{uptime(snap.uptime_s, t)}</p>
            </div>
            <div className="bg-card/45 px-3 py-2">
              <p className="micro">{t("server.tempMax")}</p>
              <p className={`mt-1 font-mono text-[12px] ${tempMax > 80 ? "text-warn" : ""}`}>
                {snap.temps.length > 0 ? `${tempMax.toFixed(1)} °C` : t("server.tempUnavailable")}
              </p>
            </div>
          </div>
        </Section>
      </div>

      <Section title={t("server.disks")} description={t("server.disksDesc")} actions={<span className="tnum font-mono text-[9px] text-muted-foreground">{snap.disks.length}</span>}>
        <div className="border border-border" role="table" aria-label={t("server.disks")}>
          <div className="hidden grid-cols-[minmax(0,1fr)_7rem_7rem_9rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 sm:grid" aria-hidden>
            <span className="micro">{t("server.mount")}</span>
            <span className="micro">{t("server.fs")}</span>
            <span className="micro text-right">{t("server.free")}</span>
            <span className="micro text-right">{t("server.used")}</span>
          </div>
          {snap.disks.map((d) => (
            <div key={d.mount} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 px-3 py-2 last:border-0 sm:grid-cols-[minmax(0,1fr)_7rem_7rem_9rem] sm:gap-3">
              <span className="flex min-w-0 items-center gap-2">
                <HardDrive className="h-3 w-3 shrink-0 text-muted-foreground/60" />
                <span className="truncate font-mono text-[11px]">{d.mount}</span>
              </span>
              <span className="hidden font-mono text-[10px] text-muted-foreground sm:block">{d.fs_type || "—"}</span>
              <span className="tnum hidden text-right font-mono text-[10px] text-muted-foreground sm:block">{bytes(d.free)}</span>
              <span className={`tnum text-right font-mono text-[10px] ${d.percent > 85 ? "text-warn" : "text-muted-foreground"}`}>
                {d.percent.toFixed(0)}% · {bytes(d.used)}
              </span>
            </div>
          ))}
          {snap.disks.length === 0 && <p className="px-3 py-3 font-mono text-[10px] text-muted-foreground">{t("server.noDisks")}</p>}
        </div>
      </Section>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Section title={t("server.temps")} description={t("server.tempsDesc")} actions={<Thermometer className="h-3.5 w-3.5 text-muted-foreground/60" />}>
          <div className="border border-border">
            {snap.temps.map((x) => (
              <div key={x.zone} className="flex items-center gap-2 border-b border-border/60 px-3 py-1.5 last:border-0">
                <span className="min-w-0 flex-1 truncate font-mono text-[10px] text-muted-foreground">{x.zone}</span>
                <span className={`tnum font-mono text-[11px] ${x.celsius > 80 ? "text-warn" : "text-foreground"}`}>{x.celsius.toFixed(1)} °C</span>
              </div>
            ))}
            {snap.temps.length === 0 && <p className="px-3 py-3 font-mono text-[10px] text-muted-foreground">{t("server.tempUnavailable")}</p>}
          </div>
        </Section>

        <Section title={t("server.firewall")} description={t("server.firewallDesc")} actions={<ShieldCheck className={`h-3.5 w-3.5 ${snap.firewall.enabled ? "text-ok" : "text-muted-foreground/60"}`} />}>
          {snap.firewall.available ? (
            <div className="border border-border">
              <div className="flex items-center gap-2 border-b border-border/60 px-3 py-1.5">
                <span className="micro">{t("server.state")}</span>
                <span className={`ml-auto font-mono text-[10px] uppercase ${snap.firewall.enabled ? "text-ok" : "text-warn"}`}>
                  {snap.firewall.enabled ? t("server.enabled") : t("server.disabled")}
                </span>
              </div>
              {(snap.firewall.profiles ?? []).map((p) => (
                <p key={p} className="border-b border-border/60 px-3 py-1.5 font-mono text-[10px] text-muted-foreground last:border-0">{p}</p>
              ))}
              {snap.firewall.detail && <p className="truncate px-3 py-1.5 font-mono text-[9px] text-muted-foreground/70">{snap.firewall.detail}</p>}
            </div>
          ) : (
            <p className="border border-border bg-card/30 px-3 py-3 font-mono text-[10px] text-muted-foreground">
              {snap.firewall.detail || t("server.firewallUnavailable")}
            </p>
          )}
        </Section>
      </div>

      <Section title={t("server.processes")} description={t("server.processesDesc")} actions={<Activity className="h-3.5 w-3.5 text-muted-foreground/60" />}>
        <div className="border border-border" role="table" aria-label={t("server.processes")}>
          <div className="hidden grid-cols-[4rem_minmax(0,1fr)_6rem_6rem_5rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 sm:grid" aria-hidden>
            <span className="micro">PID</span>
            <span className="micro">{t("server.process")}</span>
            <span className="micro text-right">CPU</span>
            <span className="micro text-right">RAM</span>
            <span className="micro text-right">{t("server.threads")}</span>
          </div>
          {procs.map((p) => (
            <div key={p.pid} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 px-3 py-1.5 last:border-0 sm:grid-cols-[4rem_minmax(0,1fr)_6rem_6rem_5rem] sm:gap-3">
              <span className="tnum font-mono text-[10px] text-muted-foreground">{p.pid}</span>
              <span className="truncate font-mono text-[11px]">{p.name}</span>
              <span className="tnum hidden text-right font-mono text-[10px] text-muted-foreground sm:block">{p.cpu_pct.toFixed(1)}%</span>
              <span className="tnum text-right font-mono text-[10px] text-muted-foreground sm:hidden">
                {p.cpu_pct.toFixed(1)}% · {p.rss_mb.toFixed(1)} MB
              </span>
              <span className="tnum hidden text-right font-mono text-[10px] text-muted-foreground sm:block">{p.rss_mb.toFixed(1)} MB</span>
              <span className="tnum hidden text-right font-mono text-[10px] text-muted-foreground sm:block">{p.threads ?? "—"}</span>
            </div>
          ))}
          {procs.length === 0 && <p className="px-3 py-3 font-mono text-[10px] text-muted-foreground">{t("server.noProcesses")}</p>}
        </div>
      </Section>

      {(snap.warnings ?? []).length > 0 && (
        <div className="border border-warn/30 bg-warn/5 px-3 py-2">
          {(snap.warnings ?? []).map((w) => (
            <p key={w} className="font-mono text-[10px] text-warn">{w}</p>
          ))}
        </div>
      )}
    </div>
  );
}
