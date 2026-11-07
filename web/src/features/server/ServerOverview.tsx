import { Bars } from "../../shared/components/feedback";
import { Stat, StatGrid, Section } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import type { ServerPoint, ServerSnapshot } from "./api";
import { bytes, rate } from "./format";

// Cabecera numérica + históricos de CPU/RAM. Solo props: reutilizable.
export function ServerOverview({ snap, history }: { snap: ServerSnapshot; history: ServerPoint[] }) {
  const { t } = useI18n();
  const diskMax = snap.disks.reduce((m, d) => Math.max(m, d.percent), 0);
  const netRate = snap.network.rx_rate + snap.network.tx_rate;
  const labels = history.map((p, i) => (history.length > 8 && i % 2 === 1 ? "" : p.ts.slice(11, 16)));
  return (
    <>
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
    </>
  );
}
