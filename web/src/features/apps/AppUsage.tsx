import { useCallback, useEffect, useState } from "react";
import { useParams } from "react-router";
import { Activity, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { getUsage } from "./api";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";
import { Section, Stat, StatGrid } from "../../shared/components/patterns";
import { bytes, uptime } from "../../shared/lib/format";
import { AppUsage as Usage } from "../../shared/lib/types";
import { useI18n } from "../../shared/i18n";

// AppUsage: consumo medido del proceso + disco real + tráfico del access log.
// Sondea cada 5s mientras la pestaña está montada.
export function AppUsage() {
  const { t, tn } = useI18n();
  const { name = "" } = useParams();
  const fail = useUI((u) => u.fail);
  const online = useUI((u) => u.online);
  const { metas } = useFleet();
  const [u, setU] = useState<Usage | null>(null);
  const [at, setAt] = useState<Date | null>(null);
  const [busy, setBusy] = useState(false);
  const [auto, setAuto] = useState(true);

  const load = useCallback(() => {
    if (!name) return;
    setBusy(true);
    getUsage(name)
      .then((r) => {
        setU(r);
        setAt(new Date());
      })
      .catch(fail)
      .finally(() => setBusy(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name]);

  useEffect(() => {
    if (!online) return;
    load();
    if (!auto) return;
    const id = window.setInterval(load, 5000);
    return () => window.clearInterval(id);
  }, [online, auto, load]);

  const meta = metas.find((m) => m.name === name);
  const limit = meta?.memory_mb ?? 0;
  const rssPct = limit > 0 && u ? Math.min(100, (u.rss_mb / limit) * 100) : 0;

  return (
    <div className="space-y-4">
      <Section
        title={t("apps.usage.title")}
        accent
        description={t("apps.usage.desc")}
        actions={
          <div className="flex items-center gap-2">
            {at && (
              <span className="tnum font-mono text-[9px] text-muted-foreground">
                {at.toLocaleTimeString("es-MX", { hour12: false })}
              </span>
            )}
            <Button
              variant={auto ? "outline" : "ghost"}
              size="xs"
              onClick={() => setAuto((a) => !a)}
              title={auto ? t("apps.usage.stopPolling") : t("apps.usage.resumePolling")}
            >
              <Activity className="h-3 w-3" /> {auto ? t("apps.usage.live") : t("apps.usage.paused")}
            </Button>
            <Button variant="ghost" size="xs" onClick={load} disabled={busy} aria-label={t("apps.usage.measureAria")}>
              <RefreshCw className={busy ? "h-3 w-3 animate-spin" : "h-3 w-3"} /> {t("apps.usage.measure")}
            </Button>
          </div>
        }
      >
        {!u && (
          <p className="bg-card/30 px-3 py-8 text-center font-mono text-[10px] text-muted-foreground">
            {online ? t("apps.usage.measuring") : t("apps.usage.offline")}
          </p>
        )}

        {u && !u.running && (
          <p className="mb-3 border border-border bg-card/30 px-3 py-2 font-mono text-[10px] text-muted-foreground">
            {t("apps.usage.stopped")}
          </p>
        )}

        {u && (
          <>
            <StatGrid>
              <Stat
                value={`${u.running ? u.cpu_pct.toFixed(1) : "0.0"}%`}
                label="cpu"
                sub={t("apps.usage.cpuSub")}
                tone={u.cpu_pct > 80 ? "warn" : "ok"}
              />
              <Stat
                value={`${u.rss_mb.toFixed(1)} MB`}
                label="ram"
                sub={limit > 0 ? t("apps.usage.ramLimit", { limit }) : t("apps.usage.ramPeak", { peak: u.peak_rss_mb.toFixed(1) })}
                tone={limit > 0 && rssPct > 85 ? "bad" : "ok"}
              />
              <Stat value={uptime(u.uptime_s)} label="uptime" sub={tn("apps.usage.restarts", u.restarts)} />
              <Stat value={String(u.handles)} label="handles" sub={tn("apps.usage.threads", u.threads)} />
            </StatGrid>

            {limit > 0 && (
              <div className="mt-3">
                <div className="flex items-center justify-between font-mono text-[9px] text-muted-foreground">
                  <span>{t("apps.usage.ramVsLimit")}</span>
                  <span className="tnum">
                    {u.rss_mb.toFixed(1)} / {limit} MB
                  </span>
                </div>
                <div className="mt-1 h-1.5 w-full bg-secondary">
                  <div
                    className={rssPct > 85 ? "h-full bg-bad" : "h-full bg-signal"}
                    style={{ width: `${Math.max(1, rssPct)}%` }}
                  />
                </div>
              </div>
            )}

            <StatGrid className="mt-3">
              <Stat value={bytes(u.disk_bytes)} label={t("apps.usage.disk")} sub={u.disk_partial ? t("apps.usage.diskPartial") : t("apps.usage.diskFull")} />
              <Stat value={bytes(u.log_bytes)} label={t("apps.usage.log")} sub={t("apps.usage.logSub")} />
              <Stat value={bytes(u.io_read_mb * 1048576)} label={t("apps.usage.ioRead")} sub={t("apps.usage.ioSub")} />
              <Stat value={bytes(u.io_write_mb * 1048576)} label={t("apps.usage.ioWrite")} sub={t("apps.usage.ioSub")} />
            </StatGrid>

            <StatGrid className="mt-3">
              <Stat value={String(u.traffic.requests)} label={t("apps.usage.requests", { window: u.traffic.window_s })} />
              <Stat value={bytes(u.traffic.bytes)} label={t("apps.usage.bandwidth")} sub={`${u.traffic.rps.toFixed(2)} req/s`} />
              <Stat
                value={`${u.traffic.errors}`}
                label={t("apps.usage.errors5xx")}
                sub={`${u.traffic.err_pct.toFixed(1)}%`}
                tone={u.traffic.errors > 0 ? "warn" : undefined}
              />
              <Stat value={u.pid ? String(u.pid) : "—"} label="pid" sub={u.http_ok ? "http ok" : t("apps.usage.noResponse")} />
            </StatGrid>

            {u.traffic.requests === 0 && (
              <p className="mt-3 font-mono text-[9px] text-muted-foreground/70">
                {t("apps.usage.noTraffic")}
              </p>
            )}
          </>
        )}
      </Section>
    </div>
  );
}
