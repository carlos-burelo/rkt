import { useEffect, useState } from "react";
import { History, Pencil, Plus } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useI18n } from "../../shared/i18n";
import { listFires, type AlertChannel, type AlertFire, type AlertRule, type RuleIn } from "./api";
import { useAlerts } from "./alerts.store";

export const KINDS = ["failed", "error_rate", "latency_p95", "memory_mb", "down_min", "disk_pct", "cpu_pct", "mem_pct", "temp_c", "cert_days", "deploy_failed"] as const;

// Campos de regla reutilizados por crear y editar (formulario local).
export function RuleFields({
  app, setApp, kind, setKind, threshold, setThreshold, windowMin, setWindowMin, channel, setChannel, channels,
}: {
  app: string; setApp: (v: string) => void;
  kind: string; setKind: (v: string) => void;
  threshold: string; setThreshold: (v: string) => void;
  windowMin: string; setWindowMin: (v: string) => void;
  channel: string; setChannel: (v: string) => void;
  channels: AlertChannel[];
}) {
  const { t } = useI18n();
  return (
    <>
      <Field label={t("ops.alerts.app")} hint={t("ops.alerts.appHint")}>
        <Input value={app} onChange={(e) => setApp(e.target.value)} placeholder={t("ops.alerts.appPlaceholder")} className="h-8 font-mono text-[12px]" />
      </Field>
      <Field label={t("ops.alerts.kind")}>
        <Select value={kind} onValueChange={(v: string | null) => setKind(v ?? "failed")}>
          <SelectTrigger className="h-8 font-mono text-[12px]">
            <span>{t(`ops.alerts.kindName.${kind}`)}</span>
          </SelectTrigger>
          <SelectContent>
            {KINDS.map((k) => (
              <SelectItem key={k} value={k}>{t(`ops.alerts.kindName.${k}`)}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label={t("ops.alerts.threshold")} hint={t(`ops.alerts.thresholdHint.${kind}`)}>
          <Input value={threshold} onChange={(e) => setThreshold(e.target.value)} className="tnum h-8 font-mono text-[12px]" />
        </Field>
        <Field label={t("ops.alerts.window")} hint={t("ops.alerts.windowHint")}>
          <Input value={windowMin} onChange={(e) => setWindowMin(e.target.value)} className="tnum h-8 font-mono text-[12px]" />
        </Field>
      </div>
      <Field label={t("ops.alerts.channel")}>
        <Select value={channel} onValueChange={(v: string | null) => setChannel(v ?? "0")}>
          <SelectTrigger className="h-8 font-mono text-[12px]">
            <span>{channels.find((c) => String(c.id) === channel)?.name ?? t("ops.alerts.noChannel")}</span>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="0">{t("ops.alerts.noChannel")}</SelectItem>
            {channels.map((c) => (
              <SelectItem key={c.id} value={String(c.id)}>{c.name || c.url}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
    </>
  );
}

export function RuleEdit({ rule, channels, trigger }: { rule: AlertRule; channels: AlertChannel[]; trigger?: React.ReactElement }) {
  const { t } = useI18n();
  const save = useAlerts((s) => s.save);
  const [open, setOpen] = useState(false);
  const [app, setApp] = useState(rule.app);
  const [kind, setKind] = useState(rule.kind);
  const [threshold, setThreshold] = useState(String(rule.threshold));
  const [windowMin, setWindowMin] = useState(String(rule.window_min));
  const [enabled, setEnabled] = useState(rule.enabled);
  const [channel, setChannel] = useState(String(rule.channel_id || 0));
  return (
    <FormDialog
      open={open}
      onOpenChange={setOpen}
      trigger={
        trigger ?? (
          <Button variant="ghost" size="xs" aria-label={t("ops.alerts.edit")}>
            <Pencil className="h-3 w-3" />
          </Button>
        )
      }
      title={t("ops.alerts.editTitle", { id: rule.id })}
      description={t("ops.alerts.editDesc")}
      submitLabel={t("common.save")}
      canSubmit={Number.isFinite(parseFloat(threshold)) && parseInt(windowMin, 10) > 0}
      onSubmit={async () => {
        const payload: RuleIn = {
          app: app.trim() || "*", kind, threshold: parseFloat(threshold),
          window_min: parseInt(windowMin, 10), enabled, channel_id: Number(channel),
        };
        await save(rule.id, payload);
        toast.success(t("ops.alerts.saved"));
      }}
    >
      <RuleFields
        app={app} setApp={setApp} kind={kind} setKind={setKind}
        threshold={threshold} setThreshold={setThreshold}
        windowMin={windowMin} setWindowMin={setWindowMin}
        channel={channel} setChannel={setChannel}
        channels={channels}
      />
      <label className="flex w-fit items-center gap-2 border border-border bg-card/30 px-2.5 py-1.5 text-[11px] text-muted-foreground">
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        {t("ops.alerts.enabled")}
      </label>
    </FormDialog>
  );
}

export function FireHistory({ rule, trigger }: { rule: AlertRule; trigger?: React.ReactElement }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [fires, setFires] = useState<AlertFire[]>([]);
  useEffect(() => {
    if (!open) return;
    listFires(rule.id).then(setFires).catch(() => setFires([]));
  }, [open, rule.id]);
  return (
    <FormDialog
      open={open}
      onOpenChange={setOpen}
      trigger={
        trigger ?? (
          <Button variant="ghost" size="xs" aria-label={t("ops.alerts.history")}>
            <History className="h-3 w-3" />
          </Button>
        )
      }
      title={t("ops.alerts.historyTitle", { id: rule.id })}
      description={t("ops.alerts.historyDesc")}
      submitLabel={t("common.close")}
      onSubmit={() => setOpen(false)}
    >
      <div className="max-h-72 overflow-auto border border-border">
        {fires.map((f) => (
          <div key={f.id} className="border-b border-border/60 px-2 py-1.5 last:border-0">
            <p className="font-mono text-[10px] text-muted-foreground">{f.ts.replace("T", " ").slice(0, 19)}</p>
            <p className="font-mono text-[11px]">{f.message}</p>
          </div>
        ))}
        {fires.length === 0 && <p className="px-2 py-4 font-mono text-[10px] text-muted-foreground">{t("ops.alerts.noFires")}</p>}
      </div>
    </FormDialog>
  );
}

export function NewRuleDialog() {
  const { t } = useI18n();
  const add = useAlerts((s) => s.add);
  const channels = useAlerts((s) => s.channels);
  const [app, setApp] = useState("*");
  const [kind, setKind] = useState("failed");
  const [threshold, setThreshold] = useState("1");
  const [windowMin, setWindowMin] = useState("60");
  const [channel, setChannel] = useState("0");
  return (
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
      onSubmit={() =>
        add({
          app: app.trim() || "*", kind, threshold: parseFloat(threshold),
          window_min: parseInt(windowMin, 10), channel_id: Number(channel),
        })
      }
    >
      <RuleFields
        app={app} setApp={setApp} kind={kind} setKind={setKind}
        threshold={threshold} setThreshold={setThreshold}
        windowMin={windowMin} setWindowMin={setWindowMin}
        channel={channel} setChannel={setChannel}
        channels={channels}
      />
    </FormDialog>
  );
}
