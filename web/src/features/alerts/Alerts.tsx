import { useEffect, useState } from "react";
import { Plus } from "lucide-react";
import { listRules, addRule, delRule, AlertRule } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { Dot, StateRail } from "../../shared/components/ui";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";

export function Alerts() {
  const { t, tn } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [app, setApp] = useState("*");
  const [kind, setKind] = useState("failed");
  const [threshold, setThreshold] = useState("1");
  const [windowMin, setWindowMin] = useState("60");

  async function load() {
    try {
      setRules(await listRules());
    } catch (e) {
      fail(e);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
        actions={
          canWrite ? (
            <FormDialog
              trigger={
                <Button size="sm">
                  <Plus className="h-3 w-3" /> {t("ops.alerts.new")}
                </Button>
              }
              title={t("ops.alerts.createTitle")}
              description={t("ops.alerts.createDesc")}
              submitLabel={t("common.create")}
              canSubmit={Number.isFinite(parseFloat(threshold)) && parseInt(windowMin, 10) > 0}
              onSubmit={async () => {
                await addRule(app.trim() || "*", kind, parseFloat(threshold), parseInt(windowMin, 10));
                await load();
              }}
            >
              <Field label={t("ops.alerts.app")} hint={t("ops.alerts.appHint")}>
                <Input
                  value={app}
                  onChange={(e) => setApp(e.target.value)}
                  placeholder={t("ops.alerts.appPlaceholder")}
                  className="h-8 font-mono text-[12px]"
                  autoFocus
                />
              </Field>
              <Field label={t("ops.alerts.kind")}>
                <Select value={kind} onValueChange={(v: string | null) => setKind(v ?? "failed")}>
                  <SelectTrigger className="h-8 font-mono text-[12px]">
                    <span>{kind}</span>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="failed">{t("ops.alerts.kindFailed")}</SelectItem>
                    <SelectItem value="error_rate">{t("ops.alerts.kindErrorRate")}</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field
                  label={t("ops.alerts.threshold")}
                  hint={kind === "failed" ? t("ops.alerts.thresholdFailures") : t("ops.alerts.thresholdHint")}
                >
                  <Input
                    value={threshold}
                    onChange={(e) => setThreshold(e.target.value)}
                    placeholder={kind === "failed" ? "1" : "0.1"}
                    className="tnum h-8 font-mono text-[12px]"
                  />
                </Field>
                <Field label={t("ops.alerts.window")} hint={t("ops.alerts.windowHint")}>
                  <Input
                    value={windowMin}
                    onChange={(e) => setWindowMin(e.target.value)}
                    placeholder="60"
                    className="tnum h-8 font-mono text-[12px]"
                  />
                </Field>
              </div>
            </FormDialog>
          ) : undefined
        }
      />
      <div className="border border-border" role="table" aria-label={t("ops.alerts.tableAria")}>
        <div
          className="hidden grid-cols-[1.5rem_3rem_minmax(0,1fr)_7rem_8rem_9rem_4rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 md:grid"
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
          <div
            key={r.id}
            className="relative grid grid-cols-[1.5rem_2.5rem_minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 py-2.5 pr-3 pl-3 last:border-0 md:grid-cols-[1.5rem_3rem_minmax(0,1fr)_7rem_8rem_9rem_4rem] md:gap-3"
          >
            <StateRail status={r.last_fired ? "degraded" : "stopped"} />
            <Dot status={r.last_fired ? "degraded" : "stopped"} />
            <span className="tnum font-mono text-[11px] text-muted-foreground">#{r.id}</span>
            <div className="min-w-0">
              <p className="truncate font-mono text-[12px]">{r.app}</p>
              <p className="tnum mt-0.5 font-mono text-[10px] text-muted-foreground md:hidden">
                {r.kind} · ≥{r.threshold}/{r.window_min}min ·{" "}
                {r.last_fired ? r.last_fired.slice(0, 16).replace("T", " ") : t("ops.alerts.neverFired")}
              </p>
            </div>
            <span className="hidden font-mono text-[11px] text-muted-foreground md:block">{r.kind}</span>
            <span className="tnum hidden font-mono text-[11px] text-muted-foreground md:block">
              ≥{r.threshold}/{r.window_min}min
            </span>
            {r.last_fired ? (
              <span className="tnum hidden text-right font-mono text-[11px] text-warn md:block">
                {r.last_fired.slice(0, 16).replace("T", " ")}
              </span>
            ) : (
              <span className="hidden text-right font-mono text-[11px] text-muted-foreground md:block">{t("ops.alerts.neverFired")}</span>
            )}
            <div className="flex justify-end">
              {canWrite && (
                <ConfirmButton
                  title={t("ops.alerts.deleteTitle", { id: r.id })}
                  onConfirm={() => {
                    delRule(r.id).then(load).catch(fail);
                  }}
                />
              )}
            </div>
          </div>
        ))}
        {rules.length === 0 && (
          <EmptyState title={t("ops.alerts.emptyTitle")} hint={t("ops.alerts.emptyHint")} />
        )}
      </div>
    </div>
  );
}
