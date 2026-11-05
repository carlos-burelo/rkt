import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import { Globe2, Pencil, Plus, Upload } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Section, EnvSecret } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { getEnvValue, importAppEnv, patchAppEnv } from "./api";
import { useEnv } from "./hooks";
import { getGlobalValue } from "../env/api";
import { useGlobalEnv } from "../env/hooks";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";

function EnvEdit({ app, envKey, onSaved }: { app: string; envKey: string; onSaved: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    if (!open || loaded) return;
    getEnvValue(app, envKey)
      .then((v) => {
        setValue(v);
        setLoaded(true);
      })
      .catch(() => {});
  }, [open, loaded, app, envKey]);
  return (
    <FormDialog
      open={open}
      onOpenChange={(o) => {
        setOpen(o);
        if (!o) setLoaded(false);
      }}
      trigger={
        <Button variant="ghost" size="xs" aria-label={t("apps.env.editAria", { key: envKey })}>
          <Pencil className="h-3 w-3" />
        </Button>
      }
      title={t("apps.env.editTitle", { key: envKey })}
      description={t("apps.env.editDesc", { app })}
      submitLabel={t("common.save")}
      canSubmit={loaded}
      onSubmit={async () => {
        await patchAppEnv(app, { [envKey]: value });
        toast.success(t("apps.env.applied"));
        onSaved();
      }}
    >
      <Field label={envKey} hint={t("apps.env.valueHint")}>
        <Input value={value} onChange={(e) => setValue(e.target.value)} className="h-8 font-mono text-[12px]" autoFocus />
      </Field>
    </FormDialog>
  );
}

function EnvImport({ app, onSaved }: { app: string; onSaved: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [raw, setRaw] = useState("");
  return (
    <FormDialog
      open={open}
      onOpenChange={setOpen}
      trigger={
        <Button size="xs" variant="outline">
          <Upload className="h-3 w-3" /> {t("apps.env.import")}
        </Button>
      }
      title={t("apps.env.importTitle")}
      description={t("apps.env.importDesc", { app })}
      submitLabel={t("apps.env.importSubmit")}
      canSubmit={raw.trim() !== ""}
      onSubmit={async () => {
        const keys = await importAppEnv(app, raw);
        toast.success(t("apps.env.imported", { n: keys.length }));
        setRaw("");
        onSaved();
      }}
    >
      <Field label=".env" hint={t("apps.env.importHint")}>
        <textarea
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          rows={8}
          spellCheck={false}
          className="min-h-40 w-full resize-y border border-input bg-transparent px-2 py-1.5 font-mono text-[11px] outline-none focus-visible:border-ring"
          autoFocus
        />
      </Field>
    </FormDialog>
  );
}

// AppEnv: globales heredadas (solo lectura) y propias del proyecto. Los
// valores se revelan por clave; nunca llegan en las listas.
export function AppEnv() {
  const { t, tn } = useI18n();
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const { keys, reload } = useEnv(name);
  const global = useGlobalEnv();
  const fail = useUI((u) => u.fail);
  const { canWrite } = useAuth();
  const [key, setKey] = useState("");
  const [val, setVal] = useState("");
  const globalKeys = global.keys;

  return (
    <div className="space-y-4">
      <Section
        title={t("apps.env.globalTitle")}
        description={t("apps.env.globalDesc", { environment: t("nav.environment") })}
        actions={
          <div className="flex items-center gap-2">
            <span className="tnum font-mono text-[9px] text-muted-foreground">{tn("apps.env.vars", globalKeys.length)}</span>
            <Button variant="ghost" size="xs" onClick={() => navigate("/environment")}>
              {t("apps.env.editGlobal", { environment: t("nav.environment") })}
            </Button>
          </div>
        }
      >
        <div className="border border-border">
          {globalKeys.map((k, i) => {
            const overridden = keys.includes(k);
            return (
              <div key={k} className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-2.5 py-1.5 last:border-0">
                <span className="folio w-5 shrink-0">{String(i + 1).padStart(2, "0")}</span>
                <Globe2 className="h-3 w-3 shrink-0 text-muted-foreground/60" aria-label={t("apps.env.globalAria")} />
                <span className="w-44 shrink-0 truncate font-mono text-[11px]">{k}</span>
                <EnvSecret load={() => getGlobalValue(k)} />
                <span
                  className={
                    overridden
                      ? "shrink-0 border border-warn/40 px-1 font-mono text-[9px] text-warn"
                      : "shrink-0 font-mono text-[9px] text-muted-foreground/60"
                  }
                  title={overridden ? t("apps.env.overriddenTitle") : t("apps.env.inheritedTitle")}
                >
                  {overridden ? t("apps.env.overridden") : t("apps.env.inherited")}
                </span>
              </div>
            );
          })}
          {globalKeys.length === 0 && (
            <p className="bg-card/30 px-3 py-4 text-center font-mono text-[10px] text-muted-foreground">
              {t("apps.env.globalEmpty", { environment: t("nav.environment") })}
            </p>
          )}
        </div>
      </Section>

      <Section
        title={t("apps.env.projectTitle")}
        description={t("apps.env.projectDesc", { role: t("role.operator") })}
        actions={
          <div className="flex items-center gap-2">
            <span className="tnum font-mono text-[9px] text-muted-foreground">{tn("apps.env.vars", keys.length)}</span>
            {canWrite && <EnvImport app={name} onSaved={reload} />}
            {canWrite && (
              <FormDialog
                trigger={
                  <Button size="xs" variant="outline">
                    <Plus className="h-3 w-3" /> {t("apps.env.newVar")}
                  </Button>
                }
                title={t("apps.env.newTitle")}
                description={t("apps.env.newDesc", { app: name })}
                submitLabel={t("common.create")}
                canSubmit={key.trim() !== ""}
                onSubmit={async () => {
                  await patchAppEnv(name, { [key.trim().toUpperCase()]: val });
                  setKey("");
                  setVal("");
                  toast.success(t("apps.env.applied"));
                  reload();
                }}
              >
                <Field label={t("apps.env.key")} hint={t("apps.env.keyHint")}>
                  <Input
                    value={key}
                    onChange={(e) => setKey(e.target.value.toUpperCase())}
                    placeholder={t("apps.env.keyPlaceholder")}
                    className="h-8 font-mono text-[12px]"
                    autoFocus
                  />
                </Field>
                <Field label={t("apps.env.value")} hint={t("apps.env.valueHint")}>
                  <Input
                    value={val}
                    onChange={(e) => setVal(e.target.value)}
                    placeholder={t("apps.env.value")}
                    className="h-8 font-mono text-[12px]"
                  />
                </Field>
              </FormDialog>
            )}
          </div>
        }
      >
        <div className="border border-border">
          {keys.map((k, i) => (
            <div key={k} className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-2.5 py-1.5 last:border-0">
              <span className="folio w-5 shrink-0">{String(i + 1).padStart(2, "0")}</span>
              <span className="w-44 shrink-0 truncate font-mono text-[11px]">{k}</span>
              <EnvSecret load={() => getEnvValue(name, k)} />
              {globalKeys.includes(k) && (
                <span className="shrink-0 border border-warn/40 px-1 font-mono text-[9px] text-warn">{t("apps.env.overridesGlobal")}</span>
              )}
              {canWrite && (
                <>
                  <EnvEdit app={name} envKey={k} onSaved={reload} />
                  <Button variant="destructive" size="xs" onClick={() => patchAppEnv(name, {}, [k]).then(reload).catch(fail)}>
                    {t("common.delete")}
                  </Button>
                </>
              )}
            </div>
          ))}
          {keys.length === 0 && (
            <p className="bg-card/30 px-3 py-5 text-center font-mono text-[10px] text-muted-foreground">
              {t("apps.env.projectEmpty")}
            </p>
          )}
        </div>
      </Section>
    </div>
  );
}
