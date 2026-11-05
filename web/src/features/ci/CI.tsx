import { Deployments } from "../deployments/Deployments";
import { PageHeader, Stat, StatGrid } from "../../shared/components/patterns";
import { useFleet } from "../overview/hooks";
import { useI18n } from "../../shared/i18n";

// CI pipeline vivo: en curso primero, luego últimos. Tablero de salidas,
// misma gramática que Deployments (columnas fijas, duraciones tabulares).
export function CI() {
  const { t, tn } = useI18n();
  const { deps } = useFleet();
  const running = deps.filter((d) => d.status === "running").length;
  const ok = deps.filter((d) => d.status === "ok").length;
  const failed = deps.filter((d) => d.status === "failed").length;

  return (
    <div className="space-y-5">
      <PageHeader
        title={t("ops.ci.title")}
        description={t("ops.ci.description")}
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{tn("ops.ci.outputs", deps.length)}</span>}
      />
      <StatGrid>
        <Stat value={`${running}`} label={t("ops.ci.running")} sub={t("ops.ci.now")} tone={running > 0 ? "signal" : undefined} />
        <Stat value={`${ok}`} label={t("ops.ci.ok")} sub={t("ops.ci.history")} tone={ok > 0 ? "ok" : undefined} />
        <Stat value={`${failed}`} label={t("ops.ci.failed")} sub={t("ops.ci.needLook")} tone={failed > 0 ? "bad" : undefined} />
        <Stat value={`${deps.length}`} label={t("ops.ci.outputsLabel")} sub={t("ops.ci.prodOnly")} />
      </StatGrid>

      <section aria-label={t("ops.ci.runningAria")}>
        <div className="mb-2 flex items-end justify-between gap-3">
          <div>
            <h3 className="text-[13px] font-medium tracking-tight">{t("ops.ci.runningTitle")}</h3>
            <p className="mt-0.5 font-mono text-[9px] text-muted-foreground">{t("ops.ci.runningSub")}</p>
          </div>
          <span className="tnum font-mono text-[9px] text-muted-foreground">{tn("ops.ci.active", running)}</span>
        </div>
        <Deployments bare status="running" />
      </section>

      <section aria-label={t("ops.ci.latestAria")}>
        <div className="mb-2 flex items-end justify-between gap-3">
          <div>
            <h3 className="text-[13px] font-medium tracking-tight">{t("ops.ci.latestTitle")}</h3>
            <p className="mt-0.5 font-mono text-[9px] text-muted-foreground">{t("ops.ci.latestSub")}</p>
          </div>
          <span className="tnum font-mono text-[9px] text-muted-foreground">{tn("ops.ci.records", Math.min(20, deps.length))}</span>
        </div>
        <Deployments bare max={20} />
      </section>
    </div>
  );
}
