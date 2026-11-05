import { useEffect, useState } from "react";
import { ArrowRight, Globe2, Lock, Pencil, Plus, RotateCw, Search, Upload } from "lucide-react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { appAction, getEnvValue } from "../apps/api";
import { getGlobalValue, importGlobalEnv, patchGlobalEnv } from "./api";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Avatar } from "../../shared/components/ui";
import { PageHeader, EmptyState, Section, EnvSecret } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useGlobalEnv } from "./hooks";
import { useI18n } from "../../shared/i18n";

function GlobalEdit({ envKey, onSaved }: { envKey: string; onSaved: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    if (!open || loaded) return;
    getGlobalValue(envKey)
      .then((v) => {
        setValue(v);
        setLoaded(true);
      })
      .catch(() => {});
  }, [open, loaded, envKey]);

  return (
    <FormDialog
      open={open}
      onOpenChange={(o) => {
        setOpen(o);
        if (!o) setLoaded(false);
      }}
      trigger={
        <Button variant="ghost" size="xs" aria-label={t("serve.env.editAria", { key: envKey })}>
          <Pencil className="h-3 w-3" />
        </Button>
      }
      title={t("serve.env.editTitle", { key: envKey })}
      description={t("serve.env.editDesc")}
      submitLabel={t("common.save")}
      canSubmit={loaded}
      onSubmit={async () => {
        await patchGlobalEnv({ [envKey]: value });
        toast.success(t("serve.env.applied"));
        onSaved();
      }}
    >
      <Field label={envKey} hint={t("serve.env.valueHint")}>
        <Input value={value} onChange={(e) => setValue(e.target.value)} className="h-8 font-mono text-[12px]" autoFocus />
      </Field>
    </FormDialog>
  );
}

function GlobalImport({ onSaved }: { onSaved: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [raw, setRaw] = useState("");
  return (
    <FormDialog
      open={open}
      onOpenChange={setOpen}
      trigger={
        <Button size="xs" variant="outline">
          <Upload className="h-3 w-3" /> {t("serve.env.import")}
        </Button>
      }
      title={t("serve.env.importTitle")}
      description={t("serve.env.importDesc")}
      submitLabel={t("serve.env.importSubmit")}
      canSubmit={raw.trim() !== ""}
      onSubmit={async () => {
        const keys = await importGlobalEnv(raw);
        toast.success(t("serve.env.imported", { n: keys.length }));
        setRaw("");
        onSaved();
      }}
    >
      <Field label=".env" hint={t("serve.env.importHint")}>
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

// EnvAll: variables globales + inventario por app. Las listas traen claves;
// cada valor se revela por clave y rol operator.
export function EnvAll() {
  const { t, tn } = useI18n();
  const fail = useUI((u) => u.fail);
  const canWrite = useAuth((a) => a.canWrite);
  const navigate = useNavigate();
  const global = useGlobalEnv();
  const [q, setQ] = useState("");
  const [key, setKey] = useState("");
  const [val, setVal] = useState("");

  const globalKeys = global.keys;
  const apps = Object.keys(global.appKeys).sort();
  const ownTotal = Object.values(global.appKeys).reduce((n, keys) => n + keys.length, 0);
  const f = q.toLowerCase();
  const visible = apps.filter((app) => {
    if (f === "") return true;
    return app.toLowerCase().includes(f) || global.appKeys[app].some((k) => k.toLowerCase().includes(f));
  });

  const save = async () => {
    const k = key.trim().toUpperCase();
    if (!k) return;
    await patchGlobalEnv({ [k]: val });
    setKey("");
    setVal("");
    toast.success(t("serve.env.applied"));
    global.reload();
  };

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("nav.environment")}
        description={t("serve.env.desc")}
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {tn("serve.env.globalCount", globalKeys.length)} · {tn("serve.env.ownCount", ownTotal)} ·{" "}
            {tn("serve.env.appCount", apps.length)}
          </span>
        }
      />

      <Section
        title={t("serve.env.globalsTitle")}
        accent
        description={t("serve.env.globalsDesc")}
        actions={
          <div className="flex items-center gap-2">
            <span className="tnum font-mono text-[9px] text-muted-foreground">{tn("serve.env.vars", globalKeys.length)}</span>
            {canWrite && <GlobalImport onSaved={global.reload} />}
            {canWrite && (
              <FormDialog
                trigger={
                  <Button size="xs" variant="outline">
                    <Plus className="h-3 w-3" /> {t("serve.env.newGlobal")}
                  </Button>
                }
                title={t("serve.env.newGlobalTitle")}
                description={t("serve.env.newGlobalDesc")}
                submitLabel={t("common.create")}
                canSubmit={key.trim() !== ""}
                onSubmit={save}
              >
                <Field label={t("serve.env.key")} hint={t("serve.env.keyHint")}>
                  <Input
                    value={key}
                    onChange={(e) => setKey(e.target.value.toUpperCase())}
                    placeholder={t("serve.env.keyPlaceholder")}
                    className="h-8 font-mono text-[12px]"
                    autoFocus
                  />
                </Field>
                <Field label={t("serve.env.value")} hint={t("serve.env.valueHint")}>
                  <Input
                    value={val}
                    onChange={(e) => setVal(e.target.value)}
                    placeholder={t("serve.env.valuePlaceholder")}
                    className="h-8 font-mono text-[12px]"
                  />
                </Field>
              </FormDialog>
            )}
          </div>
        }
      >
        <div className="border border-border">
          {globalKeys.map((k, i) => (
            <div key={k} className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-2.5 py-1.5 last:border-0">
              <span className="folio w-5 shrink-0">{String(i + 1).padStart(2, "0")}</span>
              <Globe2 className="h-3 w-3 shrink-0 text-muted-foreground/60" aria-label={t("serve.env.globalAria")} />
              <span className="w-44 shrink-0 truncate font-mono text-[11px]">{k}</span>
              <EnvSecret load={() => getGlobalValue(k)} />
              {canWrite && (
                <>
                  <GlobalEdit envKey={k} onSaved={global.reload} />
                  <Button
                    variant="destructive"
                    size="xs"
                    onClick={() => patchGlobalEnv({}, [k]).then(global.reload).catch(fail)}
                  >
                    {t("common.delete")}
                  </Button>
                </>
              )}
            </div>
          ))}
          {globalKeys.length === 0 && (
            <p className="bg-card/30 px-3 py-5 text-center font-mono text-[10px] text-muted-foreground">
              {t("serve.env.globalsEmpty")}
            </p>
          )}
        </div>
      </Section>

      <div className="flex items-center gap-2 border border-border bg-card/35 p-2">
        <div className="relative min-w-0 flex-1 sm:max-w-md">
          <Search className="absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t("serve.env.searchPlaceholder")}
            className="h-7 border-transparent bg-transparent pl-8 font-mono text-[11px] hover:border-input"
          />
        </div>
        <span className="flex-1" />
        <span className="folio">{t("serve.env.hiddenValues")}</span>
      </div>

      {visible.map((app) => {
        const keys = global.appKeys[app].filter((k) => f === "" || k.toLowerCase().includes(f) || app.toLowerCase().includes(f));
        const inherited = globalKeys.filter((k) => !global.appKeys[app].includes(k)).length;
        return (
          <section key={app} className="border border-border">
            <div className="flex items-center gap-2 border-b border-border bg-secondary/20 px-3 py-2">
              <button
                onClick={() => navigate(`/app/${app}`)}
                className="flex min-w-0 flex-1 items-center gap-2.5 text-left hover:text-signal"
              >
                <Avatar name={app} />
                <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{app}</span>
                <span className="tnum font-mono text-[9px] text-muted-foreground">
                  {tn("serve.env.ownCount", global.appKeys[app].length)} · {tn("serve.env.inherited", inherited)}
                </span>
                <ArrowRight className="h-3 w-3 text-muted-foreground/40" />
              </button>
              {canWrite && (
                <Button
                  variant="ghost"
                  size="xs"
                  title={t("serve.env.restartApp", { app })}
                  onClick={() => appAction(app, "restart").then(() => toast.success(t("serve.env.restarted", { app }))).catch(fail)}
                >
                  <RotateCw className="h-3 w-3" />
                </Button>
              )}
            </div>
            {keys.map((k) => (
              <div key={k} className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-3 py-1.5 last:border-0">
                <Lock className="h-3 w-3 shrink-0 text-muted-foreground/60" aria-label={t("serve.env.secretAria")} />
                <span className="w-52 shrink-0 truncate font-mono text-[11px]">{k}</span>
                <EnvSecret load={() => getEnvValue(app, k)} />
                {globalKeys.includes(k) && (
                  <span className="border border-warn/40 px-1 font-mono text-[9px] text-warn">
                    {t("serve.env.overridesGlobal")}
                  </span>
                )}
                <span className="flex-1" />
                <span className="font-mono text-[9px] text-muted-foreground/60">{t("serve.env.editInApp")}</span>
              </div>
            ))}
            {keys.length === 0 && (
              <p className="bg-card/30 px-3 py-3 font-mono text-[10px] text-muted-foreground">{t("serve.env.emptyOwn")}</p>
            )}
          </section>
        );
      })}
      {apps.length === 0 && <EmptyState title={t("serve.env.noApps")} hint={t("serve.env.noAppsHint")} />}
      {apps.length > 0 && visible.length === 0 && (
        <EmptyState title={t("serve.env.noMatch")} hint={t("serve.env.noMatchHint", { q })} />
      )}
    </div>
  );
}
