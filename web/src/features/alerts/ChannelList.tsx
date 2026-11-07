import { useState } from "react";
import { Bell } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Section } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import { useAlerts } from "./alerts.store";

// Canales webhook: lista + alta con formulario local.
export function ChannelList() {
  const { t } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const channels = useAlerts((s) => s.channels);
  const addChannel = useAlerts((s) => s.addChannel);
  const removeChannel = useAlerts((s) => s.removeChannel);
  const [chName, setChName] = useState("");
  const [chURL, setChURL] = useState("");
  return (
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
                onConfirm={() => removeChannel(c.id)}
              />
            )}
          </div>
        ))}
        {channels.length === 0 && <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">{t("ops.alerts.noChannels")}</p>}
      </div>
    </Section>
  );
}
