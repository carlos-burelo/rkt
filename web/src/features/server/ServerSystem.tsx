import { Activity, ShieldCheck, Thermometer } from "lucide-react";
import { Section } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import type { ServerProcess, ServerSnapshot } from "./api";

// Temperaturas, firewall, procesos y avisos. Solo props: reutilizable.
export function ServerSystem({ snap, procs }: { snap: ServerSnapshot; procs: ServerProcess[] }) {
  const { t } = useI18n();
  return (
    <>
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
    </>
  );
}
