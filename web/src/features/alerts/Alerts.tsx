import { useEffect } from "react";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import { useAlerts } from "./alerts.store";
import { NewRuleDialog } from "./ruleforms";
import { RuleRow } from "./RuleRow";
import { ChannelList } from "./ChannelList";

// Alertas: layout + tabla. Estado en el store, piezas aparte.
export function Alerts() {
  const { t, tn } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const rules = useAlerts((s) => s.rules);

  useEffect(() => {
    useAlerts.getState().load();
  }, []);

  const fired = rules.filter((r) => r.last_fired).length;

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("ops.alerts.title")}
        description={t("ops.alerts.description")}
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {tn("ops.alerts.rules", rules.length)} · {tn("ops.alerts.fired", fired)}
          </span>
        }
        actions={canWrite ? <NewRuleDialog /> : undefined}
      />
      <div className="border border-border" role="table" aria-label={t("ops.alerts.tableAria")}>
        <div
          className="hidden grid-cols-[1.5rem_3rem_minmax(0,1fr)_8rem_8rem_9rem_10rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 md:grid"
          aria-hidden
        >
          <span className="micro">{t("ops.alerts.col.state")}</span>
          <span className="micro">{t("ops.alerts.col.folio")}</span>
          <span className="micro">{t("ops.alerts.col.rule")}</span>
          <span className="micro">{t("ops.alerts.col.kind")}</span>
          <span className="micro">{t("ops.alerts.col.threshold")}</span>
          <span className="micro text-right">{t("ops.alerts.col.lastFired")}</span>
          <span />
        </div>
        {rules.map((r) => (
          <RuleRow key={r.id} id={r.id} />
        ))}
        {rules.length === 0 && (
          <EmptyState title={t("ops.alerts.emptyTitle")} hint={t("ops.alerts.emptyHint")} />
        )}
      </div>
      <ChannelList />
    </div>
  );
}
