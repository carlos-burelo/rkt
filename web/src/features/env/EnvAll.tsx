import { useEffect, useState } from "react";
import { ArrowRight, Globe2, Lock, Plus, Search } from "lucide-react";
import { useNavigate } from "react-router";
import { listApps, getEnv } from "../apps/api";
import { AppMeta } from "../../shared/lib/types";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Avatar } from "../../shared/components/ui";
import { PageHeader, EmptyState, Section, EnvSecret } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useGlobalEnv } from "./hooks";
import { delGlobalEnv, setGlobalEnv } from "./api";
import { useI18n } from "../../shared/i18n";

// EnvAll: variables globales + inventario por app.
// Los valores se revelan a demanda; la edición de globales vive aquí.
export function EnvAll() {
  const { t, tn } = useI18n();
  const fail = useUI((u) => u.fail);
  const canWrite = useAuth((a) => a.canWrite);
  const navigate = useNavigate();
  const global = useGlobalEnv();
  const [q, setQ] = useState("");
  const [rows, setRows] = useState<{ app: string; vars: Record<string, string> }[]>([]);
  const [key, setKey] = useState("");
  const [val, setVal] = useState("");

  useEffect(() => {
    listApps()
      .then(async (apps: AppMeta[]) => {
        const out: { app: string; vars: Record<string, string> }[] = [];
        for (const a of apps) {
          try {
            out.push({ app: a.name, vars: await getEnv(a.name) });
          } catch (e) {
            fail(e);
            return;
          }
        }
        setRows(out);
      })
      .catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const f = q.toLowerCase();
  const globalKeys = Object.keys(global.env).sort();
  const ownTotal = rows.reduce((n, r) => n + Object.keys(r.vars).length, 0);
  const visible = rows.filter(({ app, vars }) => {
    if (f === "") return true;
    return app.toLowerCase().includes(f) || Object.keys(vars).some((k) => k.toLowerCase().includes(f));
  });

  const save = async () => {
    const k = key.trim();
    if (!k) return;
    await setGlobalEnv(k, val);
    setKey("");
    setVal("");
    global.reload();
  };

  const canSave = key.trim() !== "";

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("nav.environment")}
        description={t("serve.env.desc")}
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {tn("serve.env.globalCount", globalKeys.length)} ·{" "}
            {tn("serve.env.ownCount", ownTotal)} · {tn("serve.env.appCount", rows.length)}
          </span>
        }
      />

      <Section
        title={t("serve.env.globalsTitle")}
        accent
        description={t("serve.env.globalsDesc")}
        actions={
          <div className="flex items-center gap-2">
            <span className="tnum font-mono text-[9px] text-muted-foreground">
              {tn("serve.env.vars", globalKeys.length)}
            </span>
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
                canSubmit={canSave}
                onSubmit={save}
              >
                <Field label={t("serve.env.key")} hint={t("serve.env.keyHint")}>
                  <Input
                    value={key}
                    onChange={(e) => setKey(e.target.value)}
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
              <EnvSecret value={global.env[k]} />
              {canWrite && (
                <Button variant="destructive" size="xs" onClick={() => delGlobalEnv(k).then(global.reload).catch(fail)}>
                  {t("common.delete")}
                </Button>
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

      {visible.map(({ app, vars }) => {
        const keys = Object.keys(vars).filter((k) => f === "" || k.toLowerCase().includes(f) || app.toLowerCase().includes(f));
        const inherited = globalKeys.filter((k) => !(k in vars)).length;
        return (
          <section key={app} className="border border-border">
            <button
              onClick={() => navigate(`/app/${app}`)}
              className="flex w-full items-center gap-2.5 border-b border-border bg-secondary/20 px-3 py-2 text-left hover:bg-accent/40"
            >
              <Avatar name={app} />
              <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{app}</span>
              <span className="tnum font-mono text-[9px] text-muted-foreground">
                {tn("serve.env.ownCount", Object.keys(vars).length)} ·{" "}
                {tn("serve.env.inherited", inherited)}
              </span>
              <ArrowRight className="h-3 w-3 text-muted-foreground/40" />
            </button>
            {keys.map((k) => (
              <div key={k} className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-3 py-1.5 last:border-0">
                <Lock className="h-3 w-3 shrink-0 text-muted-foreground/60" aria-label={t("serve.env.secretAria")} />
                <span className="w-52 shrink-0 truncate font-mono text-[11px]">{k}</span>
                {k in global.env && (
                  <span className="border border-warn/40 px-1 font-mono text-[9px] text-warn">
                    {t("serve.env.overridesGlobal")}
                  </span>
                )}
                <span className="flex-1" />
                <span className="font-mono text-[9px] text-muted-foreground/60">{t("serve.env.editInApp")}</span>
              </div>
            ))}
            {keys.length === 0 && (
              <p className="bg-card/30 px-3 py-3 font-mono text-[10px] text-muted-foreground">
                {t("serve.env.emptyOwn")}
              </p>
            )}
          </section>
        );
      })}
      {rows.length === 0 && (
        <EmptyState title={t("serve.env.noApps")} hint={t("serve.env.noAppsHint")} />
      )}
      {rows.length > 0 && visible.length === 0 && (
        <EmptyState
          title={t("serve.env.noMatch")}
          hint={t("serve.env.noMatchHint", { q })}
        />
      )}
    </div>
  );
}
