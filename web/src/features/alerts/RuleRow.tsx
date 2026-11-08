import { Power, Send, Pencil, History, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { Dot, StateRail } from "../../shared/components/ui";
import { RowMenu } from "../../shared/components/menus";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import type { AlertRule } from "./api";
import { useAlerts } from "./alerts.store";
import { FireHistory, RuleEdit } from "./ruleforms";

// Fila de regla: lee su regla por id + acciones finas (sin re-render global).
export function RuleRow({ id }: { id: number }) {
  const { t } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const rule = useAlerts((s) => s.rules.find((r) => r.id === id));
  const channels = useAlerts((s) => s.channels);
  const toggle = useAlerts((s) => s.toggle);
  const remove = useAlerts((s) => s.remove);
  const test = useAlerts((s) => s.test);
  if (!rule) return null;
  const r: AlertRule = rule;
  return (
    <div className="relative grid grid-cols-[1.5rem_2.5rem_minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 py-2.5 pr-3 pl-3 last:border-0 md:grid-cols-[1.5rem_3rem_minmax(0,1fr)_8rem_8rem_9rem_10rem] md:gap-3">
      <StateRail status={r.enabled ? (r.last_fired ? "degraded" : "ok") : "stopped"} />
      <Dot status={r.enabled ? (r.last_fired ? "degraded" : "running") : "stopped"} />
      <span className="tnum font-mono text-[11px] text-muted-foreground">#{r.id}</span>
      <div className="min-w-0">
        <p className="truncate font-mono text-[12px]">{r.app}</p>
        <p className="tnum mt-0.5 font-mono text-[10px] text-muted-foreground md:hidden">
          {t(`ops.alerts.kindName.${r.kind}`)} · ≥{r.threshold}/{r.window_min}min
        </p>
      </div>
      <span className="hidden font-mono text-[11px] text-muted-foreground md:block">{t(`ops.alerts.kindName.${r.kind}`)}</span>
      <span className="tnum hidden font-mono text-[11px] text-muted-foreground md:block">≥{r.threshold}/{r.window_min}min</span>
      {r.last_fired ? (
        <span className="tnum hidden text-right font-mono text-[11px] text-warn md:block">
          {r.last_fired.slice(0, 16).replace("T", " ")}
        </span>
      ) : (
        <span className="hidden text-right font-mono text-[11px] text-muted-foreground md:block">{t("ops.alerts.neverFired")}</span>
      )}
      <div className="flex justify-end gap-0.5">
        {canWrite && (
          <RowMenu label={t("ops.alerts.rowActions", { id: r.id })}>
            <DropdownMenuItem onClick={() => toggle(r)}>
              <Power className="h-3.5 w-3.5" /> {r.enabled ? t("ops.alerts.disable") : t("ops.alerts.enable")}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => test(r.id).then((m) => { if (m) toast.success(m); })}>
              <Send className="h-3.5 w-3.5" /> {t("ops.alerts.test")}
            </DropdownMenuItem>
            <RuleEdit rule={r} channels={channels} trigger={<DropdownMenuItem><Pencil className="h-3.5 w-3.5" aria-hidden />{t("common.edit")}</DropdownMenuItem>} />
            <FireHistory rule={r} trigger={<DropdownMenuItem><History className="h-3.5 w-3.5" aria-hidden />{t("ops.alerts.history")}</DropdownMenuItem>} />
            <ConfirmButton
              title={t("ops.alerts.deleteTitle", { id: r.id })}
              onConfirm={() => remove(r.id)}
              icon={<Trash2 className="h-3.5 w-3.5" aria-hidden />}
            />
          </RowMenu>
        )}
      </div>
    </div>
  );
}
