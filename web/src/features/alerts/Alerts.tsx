import { useEffect, useState } from "react";
import { Bell, History, Pencil, Plus, Power, Send } from "lucide-react";
import { toast } from "sonner";
import {
  AlertChannel, AlertFire, AlertRule, RuleIn,
  addChannel, addRule, delChannel, delRule, listChannels, listFires, listRules, testRule, updateRule,
} from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader, EmptyState, Section } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { Dot, StateRail } from "../../shared/components/ui";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";

const KINDS = ["failed", "error_rate", "latency_p95", "memory_mb", "down_min", "disk_pct", "cpu_pct", "mem_pct", "temp_c"] as const;

function RuleEdit({ rule, channels, onSaved }: { rule: AlertRule; channels: AlertChannel[]; onSaved: () => void }) {
  const { t } = useI18n();
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
        <Button variant="ghost" size="xs" aria-label={t("ops.alerts.edit")}>
          <Pencil className="h-3 w-3" />
        </Button>
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
        await updateRule(rule.id, payload);
        toast.success(t("ops.alerts.saved"));
        onSaved();
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

function RuleFields({
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

function FireHistory({ rule }: { rule: AlertRule }) {
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
        <Button variant="ghost" size="xs" aria-label={t("ops.alerts.history")}>
          <History className="h-3 w-3" />
        </Button>
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

export function Alerts() {
  const { t, tn } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [channels, setChannels] = useState<AlertChannel[]>([]);
  const [app, setApp] = useState("*");
  const [kind, setKind] = useState("failed");
  const [threshold, setThreshold] = useState("1");
  const [windowMin, setWindowMin] = useState("60");
  const [channel, setChannel] = useState("0");
  const [chName, setChName] = useState("");
  const [chURL, setChURL] = useState("");

  async function load() {
    try {
      const [rs, cs] = await Promise.all([listRules(), listChannels().catch(() => [])]);
      setRules(rs);
      setChannels(cs);
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
                await addRule({
                  app: app.trim() || "*", kind, threshold: parseFloat(threshold),
                  window_min: parseInt(windowMin, 10), channel_id: Number(channel),
                });
                await load();
              }}
            >
              <RuleFields
                app={app} setApp={setApp} kind={kind} setKind={setKind}
                threshold={threshold} setThreshold={setThreshold}
                windowMin={windowMin} setWindowMin={setWindowMin}
                channel={channel} setChannel={setChannel}
                channels={channels}
              />
            </FormDialog>
          ) : undefined
        }
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
          <div
            key={r.id}
            className="relative grid grid-cols-[1.5rem_2.5rem_minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 py-2.5 pr-3 pl-3 last:border-0 md:grid-cols-[1.5rem_3rem_minmax(0,1fr)_8rem_8rem_9rem_10rem] md:gap-3"
          >
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
                <>
                  <Button
                    variant="ghost"
                    size="xs"
                    title={r.enabled ? t("ops.alerts.disable") : t("ops.alerts.enable")}
                    onClick={() => updateRule(r.id, {
                      app: r.app, kind: r.kind, threshold: r.threshold,
                      window_min: r.window_min, enabled: !r.enabled, channel_id: r.channel_id,
                    }).then(load).catch(fail)}
                  >
                    <Power className={`h-3 w-3 ${r.enabled ? "text-ok" : "text-muted-foreground"}`} />
                  </Button>
                  <Button
                    variant="ghost"
                    size="xs"
                    title={t("ops.alerts.test")}
                    onClick={() => testRule(r.id).then((m) => toast.success(m)).catch(fail)}
                  >
                    <Send className="h-3 w-3" />
                  </Button>
                  <RuleEdit rule={r} channels={channels} onSaved={load} />
                  <FireHistory rule={r} />
                  <ConfirmButton
                    title={t("ops.alerts.deleteTitle", { id: r.id })}
                    onConfirm={() => delRule(r.id).then(load).catch(fail)}
                  />
                </>
              )}
            </div>
          </div>
        ))}
        {rules.length === 0 && (
          <EmptyState title={t("ops.alerts.emptyTitle")} hint={t("ops.alerts.emptyHint")} />
        )}
      </div>

      <Section
        title={t("ops.alerts.channelsTitle")}
        description={t("ops.alerts.channelsDesc")}
        actions={
          canWrite ? (
            <FormDialog
              trigger={
                <Button size="xs" variant="outline">
                  <Bell className="h-3 w-3" /> {t("ops.alerts.channelNew")}
                </Button>
              }
              title={t("ops.alerts.channelNewTitle")}
              description={t("ops.alerts.channelNewDesc")}
              submitLabel={t("common.create")}
              canSubmit={chName.trim() !== "" && /^https?:\/\//.test(chURL)}
              onSubmit={async () => {
                await addChannel(chName.trim(), chURL.trim());
                setChName("");
                setChURL("");
                await load();
              }}
            >
              <Field label={t("ops.alerts.channelName")}>
                <Input value={chName} onChange={(e) => setChName(e.target.value)} className="h-8 font-mono text-[12px]" />
              </Field>
              <Field label="URL" hint="http(s)://host/webhook">
                <Input value={chURL} onChange={(e) => setChURL(e.target.value)} className="h-8 font-mono text-[12px]" />
              </Field>
            </FormDialog>
          ) : undefined
        }
      >
        <div className="border border-border">
          {channels.map((c) => (
            <div key={c.id} className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-3 py-2 last:border-0">
              <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{c.name || c.url}</span>
              <span className="hidden max-w-[40%] truncate font-mono text-[10px] text-muted-foreground sm:block">{c.url}</span>
              <span className={`font-mono text-[9px] uppercase ${c.enabled ? "text-ok" : "text-muted-foreground"}`}>
                {c.enabled ? t("ops.alerts.channelOn") : t("ops.alerts.channelOff")}
              </span>
              {canWrite && (
                <ConfirmButton
                  title={t("ops.alerts.channelDelete", { id: c.id })}
                  onConfirm={() => delChannel(c.id).then(load).catch(fail)}
                />
              )}
            </div>
          ))}
          {channels.length === 0 && <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">{t("ops.alerts.noChannels")}</p>}
        </div>
      </Section>
    </div>
  );
}
