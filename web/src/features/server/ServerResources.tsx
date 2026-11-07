import { HardDrive } from "lucide-react";
import { Meter, Section } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import type { ServerSnapshot } from "./api";
import { bytes, tone, uptime } from "./format";

// Memoria, carga y discos. Solo props: reutilizable.
export function ServerResources({ snap }: { snap: ServerSnapshot }) {
  const { t } = useI18n();
  const tempMax = snap.temps.reduce((m, x) => Math.max(m, x.celsius), 0);
  return (
    <>
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
    </>
  );
}
