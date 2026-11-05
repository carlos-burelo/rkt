import { AppStatus, Deployment } from "../../shared/lib/types";
import { Stat } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";

// Placas de flota a escala fija: mismo tamaño, número tabular arriba,
// etiqueta abajo, sub honesto con ventana de medición.
export function Metrics({ apps, deps }: { apps: AppStatus[]; deps: Deployment[] }) {
  const { t } = useI18n();
  const running = apps.filter((a) => a.running && !a.failed && a.http_ok !== false).length;
  const failed = apps.filter((a) => a.failed).length;
  const day = Date.now() - 24 * 3600 * 1000;
  const recent = deps.filter((d) => new Date(d.started_at).getTime() >= day);
  const done = recent.filter((d) => d.status !== "running");
  const ok = done.filter((d) => d.status === "ok").length;
  const rate = done.length === 0 ? "—" : `${Math.round((ok / done.length) * 100)}%`;

  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4" role="group" aria-label={t("overview.metrics.aria")}>
      <Stat
        value={`${running}/${apps.length}`}
        label={t("overview.metrics.running")}
        sub={t("overview.metrics.runningSub")}
        tone={failed > 0 ? "warn" : "ok"}
      />
      <Stat
        value={failed === 0 ? "0" : `${failed}`}
        label={t("overview.metrics.failed")}
        sub={failed === 0 ? t("overview.metrics.failedNone") : t("overview.metrics.failedAction")}
        tone={failed === 0 ? undefined : "bad"}
      />
      <Stat
        value={`${recent.length}`}
        label={t("overview.metrics.deploys")}
        sub={t("overview.metrics.deploysSub")}
        tone="signal"
      />
      <Stat
        value={rate}
        label={t("overview.metrics.rate")}
        sub={done.length === 0 ? t("overview.metrics.rateNone") : t("overview.metrics.rateSub", { ok, total: done.length })}
      />
    </div>
  );
}
