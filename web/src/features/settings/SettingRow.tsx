import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { PathInput } from "../../shared/components/PathPicker";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import { useSettings } from "./settings.store";

const DESCRIPTION_KEYS: Record<string, string> = {
  node_bin: "system.settings.desc.node_bin",
  python_bin: "system.settings.desc.python_bin",
  base_domain: "system.settings.desc.base_domain",
  port_pool: "system.settings.desc.port_pool",
  site_name: "system.settings.desc.site_name",
  quota_reqs: "system.settings.desc.quota_reqs",
  quota_bytes: "system.settings.desc.quota_bytes",
  public_ip: "system.settings.desc.public_ip",
};

const LABELS: Record<string, string> = {
  node_bin: "system.settings.label.node_bin",
  python_bin: "system.settings.label.python_bin",
  base_domain: "system.settings.label.base_domain",
  port_pool: "system.settings.label.port_pool",
  site_name: "system.settings.label.site_name",
  quota_reqs: "system.settings.label.quota_reqs",
  quota_bytes: "system.settings.label.quota_bytes",
  public_ip: "system.settings.label.public_ip",
};

const PICKERS: Record<string, "dir" | "file"> = {
  node_bin: "file",
  python_bin: "file",
};

// Fila de ajuste con edición inline: borrador en el store (sobrevive al
// cerrar el diálogo sin guardar).
export function SettingRow({ k, index }: { k: string; index: number }) {
  const { t } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const value = useSettings((s) => s.settings[k] ?? "");
  const draftVal = useSettings((s) => s.draft[k]);
  const setDraftKey = useSettings((s) => s.setDraftKey);
  const save = useSettings((s) => s.save);
  const resetKey = useSettings((s) => s.resetKey);
  const v = value;
  return (
    <div className="grid grid-cols-1 gap-2 border-b border-border/60 bg-card/35 px-3 py-2.5 last:border-0 md:grid-cols-[2rem_minmax(0,1fr)_18rem_6.5rem] md:items-center md:gap-3">
      <span className="folio hidden md:block">{String(index + 1).padStart(2, "0")}</span>
      <div className="min-w-0">
        <p className="font-mono text-[11px]">{t(LABELS[k] ?? k)}</p>
        <p className="mt-0.5 font-mono text-[9px] text-muted-foreground/60">{k}</p>
        {DESCRIPTION_KEYS[k] && <p className="mt-0.5 text-[10px] leading-relaxed text-muted-foreground">{t(DESCRIPTION_KEYS[k])}</p>}
      </div>
      <span className="tnum min-w-0 truncate font-mono text-[11px] text-muted-foreground" title={v}>
        {v === "" ? "—" : v}
      </span>
      {canWrite ? (
        <div className="flex justify-end gap-1">
          <FormDialog
            trigger={
              <Button variant="outline" size="xs">
                {t("common.edit")}
              </Button>
            }
            title={t("system.settings.editTitle", { key: k })}
            description={DESCRIPTION_KEYS[k] ? t(DESCRIPTION_KEYS[k]) : t("system.settings.validatedHint")}
            submitLabel={t("common.save")}
            onSubmit={() => save(k)}
          >
            <Field label={k} hint={PICKERS[k] ? t("system.settings.pathOnDisk") : undefined}>
              {PICKERS[k] ? (
                <PathInput
                  value={draftVal ?? v}
                  onChange={(nv) => setDraftKey(k, nv)}
                  mode={PICKERS[k]}
                  ariaLabel={t("system.settings.valueAria", { key: k })}
                  className="tnum h-8 min-w-0 flex-1 font-mono text-[12px]"
                />
              ) : (
                <Input
                  value={draftVal ?? v}
                  onChange={(e) => setDraftKey(k, e.target.value)}
                  id={`set-${k}`}
                  aria-label={t("system.settings.valueAria", { key: k })}
                  className="tnum h-8 font-mono text-[12px]"
                  autoFocus
                />
              )}
            </Field>
          </FormDialog>
          {v !== "" && (
            <ConfirmButton
              label={t("system.settings.reset")}
              confirmLabel={t("common.confirm")}
              title={t("system.settings.resetTitle", { key: k })}
              onConfirm={() => resetKey(k)}
            />
          )}
        </div>
      ) : (
        <span className="font-mono text-[9px] text-muted-foreground">{t("system.settings.readOnly")}</span>
      )}
    </div>
  );
}
