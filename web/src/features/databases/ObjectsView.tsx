import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Section } from "../../shared/components/patterns";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import {
  listViews, getViewDef, dropView, refreshMatview,
  listFunctions, getFunctionDef, dropFunction,
  listProcedures, getProcedureDef, callProcedure, dropProcedure,
  listTriggers, dropTrigger, toggleTrigger,
  listSequences, restartSequence, setvalSequence,
  type ViewInfo, type FuncInfo, type TriggerInfo, type SeqInfo,
} from "./api";
import { CreateViewDialog, CreateTriggerDialog } from "./TableDialogs";
import { useDb } from "./store";

// Objetos de la base (vistas, funciones, procedimientos, triggers,
// secuencias): ledger de solo lectura con mutaciones operator tras
// ConfirmButton de dos pasos. Cada sección refresca su lista.
export function ObjectsView({
  app,
  canWrite,
  onChanged,
}: {
  app: string;
  canWrite: boolean;
  onChanged: () => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const tables = useDb((s) => s.tables);
  const [views, setViews] = useState<ViewInfo[]>([]);
  const [funcs, setFuncs] = useState<FuncInfo[]>([]);
  const [procs, setProcs] = useState<FuncInfo[]>([]);
  const [triggers, setTriggers] = useState<TriggerInfo[]>([]);
  const [seqs, setSeqs] = useState<SeqInfo[]>([]);
  const [defs, setDefs] = useState<Record<string, string>>({});
  const [openDef, setOpenDef] = useState<Record<string, boolean>>({});
  const [tableFilter, setTableFilter] = useState("");
  const [callArgs, setCallArgs] = useState<Record<string, string>>({});
  const [callOut, setCallOut] = useState("");
  const [restartWith, setRestartWith] = useState<Record<string, string>>({});
  const [setvalIn, setSetvalIn] = useState<Record<string, string>>({});

  const loadViews = useCallback(async () => {
    try {
      setViews(await listViews(app));
    } catch (e) {
      fail(e);
    }
  }, [app, fail]);
  const loadFuncs = useCallback(async () => {
    try {
      setFuncs(await listFunctions(app));
    } catch (e) {
      fail(e);
    }
  }, [app, fail]);
  const loadProcs = useCallback(async () => {
    try {
      setProcs(await listProcedures(app));
    } catch (e) {
      fail(e);
    }
  }, [app, fail]);
  const loadTriggers = useCallback(async () => {
    try {
      setTriggers(await listTriggers(app));
    } catch (e) {
      fail(e);
    }
  }, [app, fail]);
  const loadSeqs = useCallback(async () => {
    try {
      setSeqs(await listSequences(app));
    } catch (e) {
      fail(e);
    }
  }, [app, fail]);

  useEffect(() => {
    setDefs({});
    setOpenDef({});
    setCallOut("");
    void loadViews();
    void loadFuncs();
    void loadProcs();
    void loadTriggers();
    void loadSeqs();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [app]);

  const toggleDef = (key: string) =>
    setOpenDef((p) => ({ ...p, [key]: !p[key] }));

  const showViewDef = async (v: ViewInfo) => {
    const key = `v:${v.name}`;
    if (openDef[key]) {
      toggleDef(key);
      return;
    }
    try {
      const d = v.def ?? (await getViewDef(app, v.name)).def ?? "";
      setDefs((p) => ({ ...p, [key]: d }));
      setOpenDef((p) => ({ ...p, [key]: true }));
    } catch (e) {
      fail(e);
    }
  };

  const showFuncDef = async (kind: "f" | "p", f: FuncInfo) => {
    const key = `${kind}:${f.identity}`;
    if (openDef[key]) {
      toggleDef(key);
      return;
    }
    try {
      let d = f.def;
      if (!d) {
        d =
          kind === "f"
            ? ((await getFunctionDef(app, f.identity)).def ?? "")
            : ((await getProcedureDef(app, f.identity)).def ?? "");
      }
      setDefs((p) => ({ ...p, [key]: d }));
      setOpenDef((p) => ({ ...p, [key]: true }));
    } catch (e) {
      fail(e);
    }
  };

  const doDropView = async (name: string) => {
    try {
      await dropView(app, name);
      await loadViews();
      onChanged();
    } catch (e) {
      fail(e);
    }
  };

  const doRefresh = async (name: string) => {
    try {
      await refreshMatview(app, name);
      toast.success(name);
      await loadViews();
    } catch (e) {
      fail(e);
    }
  };

  const doDropFunc = async (identity: string) => {
    try {
      await dropFunction(app, identity);
      await loadFuncs();
      onChanged();
    } catch (e) {
      fail(e);
    }
  };

  const doDropProc = async (identity: string) => {
    try {
      await dropProcedure(app, identity);
      await loadProcs();
      onChanged();
    } catch (e) {
      fail(e);
    }
  };

  const doCall = async (f: FuncInfo) => {
    const expr = (callArgs[f.identity] ?? `${f.name}()`).trim();
    if (!expr) return;
    try {
      const out = await callProcedure(app, expr);
      setCallOut(out.result ?? "");
      toast.success(f.name);
    } catch (e) {
      fail(e);
    }
  };

  const doDropTrigger = async (table: string, name: string) => {
    try {
      await dropTrigger(app, table, name);
      await loadTriggers();
      onChanged();
    } catch (e) {
      fail(e);
    }
  };

  const doToggle = async (tr: TriggerInfo, enable: boolean) => {
    try {
      await toggleTrigger(app, tr.table, tr.name, enable);
      await loadTriggers();
    } catch (e) {
      fail(e);
    }
  };

  const doRestart = async (s: SeqInfo) => {
    try {
      const withVal = (restartWith[s.name] ?? "").trim() || undefined;
      await restartSequence(app, s.name, withVal);
      toast.success(s.name);
      await loadSeqs();
    } catch (e) {
      fail(e);
    }
  };

  const doSetval = async (s: SeqInfo) => {
    const raw = (setvalIn[s.name] ?? "").trim();
    if (!raw) return;
    try {
      await setvalSequence(app, s.name, Number(raw));
      toast.success(s.name);
      await loadSeqs();
    } catch (e) {
      fail(e);
    }
  };

  const triggerTables = [...new Set(triggers.map((x) => x.table))].sort();
  const shownTriggers = tableFilter ? triggers.filter((x) => x.table === tableFilter) : triggers;
  const tableNames = tables.map((x) => x.name);
  const triggerDefaultTable = tableFilter || tableNames[0] || triggerTables[0] || "";

  const badge = "border border-border px-1 text-[9px] uppercase text-muted-foreground";
  const row =
    "border border-border/60 bg-card/40 px-2 py-1.5 font-mono text-[11px]";

  return (
    <div className="space-y-4">
      <Section
        title={t("db.views")}
        description={`${views.length}`}
        actions={
          canWrite ? (
            <CreateViewDialog
              app={app}
              onCreated={() => {
                void loadViews();
                onChanged();
              }}
            />
          ) : undefined
        }
      >
        {views.length === 0 ? (
          <p className="font-mono text-[11px] text-muted-foreground/70">{t("db.noViews")}</p>
        ) : (
          <ul className="space-y-1">
            {views.map((v) => {
              const key = `v:${v.name}`;
              return (
                <li key={v.name} className={row}>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="min-w-0 flex-1 truncate">{v.name}</span>
                    <span className={badge}>{v.kind}</span>
                    {v.kind === "materialized" && typeof v.has_data === "boolean" && (
                      <span className={badge}>{v.has_data ? "has_data" : "no_data"}</span>
                    )}
                    <Button
                      size="sm"
                      variant="ghost"
                      className="font-mono text-[11px] text-muted-foreground"
                      onClick={() => void showViewDef(v)}
                    >
                      {openDef[key] ? t("db.hideDef") : t("db.viewDef")}
                    </Button>
                    {canWrite && v.kind === "materialized" && (
                      <ConfirmButton
                        size="sm"
                        label={t("db.refresh")}
                        confirmLabel={t("db.refreshConfirm")}
                        onConfirm={() => void doRefresh(v.name)}
                      />
                    )}
                    {canWrite && (
                      <ConfirmButton
                        size="sm"
                        label="×"
                        confirmLabel={t("db.dropViewConfirm")}
                        onConfirm={() => void doDropView(v.name)}
                      />
                    )}
                  </div>
                  {openDef[key] && (
                    <pre className="mt-1.5 max-h-[240px] overflow-auto border border-border/60 bg-field/25 px-2 py-1.5 font-mono text-[11px] whitespace-pre-wrap text-muted-foreground">
                      {defs[key] ?? ""}
                    </pre>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </Section>

      <Section title={t("db.functions")} description={`${funcs.length}`}>
        {funcs.length === 0 ? (
          <p className="font-mono text-[11px] text-muted-foreground/70">{t("db.noFunctions")}</p>
        ) : (
          <ul className="space-y-1">
            {funcs.map((f) => {
              const key = `f:${f.identity}`;
              return (
                <li key={f.identity} className={row}>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="min-w-0 flex-1 truncate" title={f.identity}>
                      {f.name}({f.args}) → {f.return}
                    </span>
                    <span className={badge}>{f.lang}</span>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="font-mono text-[11px] text-muted-foreground"
                      onClick={() => void showFuncDef("f", f)}
                    >
                      {openDef[key] ? t("db.hideDef") : t("db.functionDef")}
                    </Button>
                    {canWrite && (
                      <ConfirmButton
                        size="sm"
                        label="×"
                        confirmLabel={t("db.dropFunctionConfirm")}
                        onConfirm={() => void doDropFunc(f.identity)}
                      />
                    )}
                  </div>
                  {openDef[key] && (
                    <pre className="mt-1.5 max-h-[240px] overflow-auto border border-border/60 bg-field/25 px-2 py-1.5 font-mono text-[11px] whitespace-pre-wrap text-muted-foreground">
                      {defs[key] ?? ""}
                    </pre>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </Section>

      <Section title={t("db.procedures")} description={`${procs.length}`}>
        {procs.length === 0 ? (
          <p className="font-mono text-[11px] text-muted-foreground/70">{t("db.noProcedures")}</p>
        ) : (
          <ul className="space-y-1">
            {procs.map((f) => {
              const key = `p:${f.identity}`;
              return (
                <li key={f.identity} className={row}>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="min-w-0 flex-1 truncate" title={f.identity}>
                      {f.name}({f.args})
                    </span>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="font-mono text-[11px] text-muted-foreground"
                      onClick={() => void showFuncDef("p", f)}
                    >
                      {openDef[key] ? t("db.hideDef") : t("db.functionDef")}
                    </Button>
                    {canWrite && (
                      <ConfirmButton
                        size="sm"
                        label="×"
                        confirmLabel={t("db.dropProcedureConfirm")}
                        onConfirm={() => void doDropProc(f.identity)}
                      />
                    )}
                  </div>
                  {openDef[key] && (
                    <pre className="mt-1.5 max-h-[240px] overflow-auto border border-border/60 bg-field/25 px-2 py-1.5 font-mono text-[11px] whitespace-pre-wrap text-muted-foreground">
                      {defs[key] ?? ""}
                    </pre>
                  )}
                  {canWrite && (
                    <div className="mt-1.5 flex flex-col gap-1.5 sm:flex-row sm:items-center">
                      <Input
                        value={callArgs[f.identity] ?? `${f.name}()`}
                        onChange={(e) =>
                          setCallArgs((p) => ({ ...p, [f.identity]: e.target.value }))
                        }
                        placeholder={t("db.callArgsHint")}
                        aria-label={t("db.call")}
                        className="h-8 flex-1 font-mono text-[11px]"
                      />
                      <ConfirmButton
                        size="sm"
                        label={t("db.call")}
                        confirmLabel={t("db.callConfirm")}
                        onConfirm={() => void doCall(f)}
                      />
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )}
        {callOut !== "" && (
          <pre className="mt-2 max-h-[160px] overflow-auto border border-border/60 bg-field/25 px-2 py-1.5 font-mono text-[11px] whitespace-pre-wrap text-ok">
            {callOut}
          </pre>
        )}
      </Section>

      <Section
        title={t("db.triggers")}
        description={`${shownTriggers.length}/${triggers.length}`}
        actions={
          canWrite && triggerDefaultTable ? (
            <CreateTriggerDialog
              key={triggerDefaultTable}
              app={app}
              table={triggerDefaultTable}
              tables={tableNames.length ? tableNames : triggerTables}
              onCreated={() => {
                void loadTriggers();
                onChanged();
              }}
            />
          ) : undefined
        }
      >
        <div className="mb-2 flex items-center gap-2">
          <label className="micro text-muted-foreground" htmlFor="obj-trigger-table">
            {t("db.triggerTable")}
          </label>
          <select
            id="obj-trigger-table"
            value={tableFilter}
            onChange={(e) => setTableFilter(e.target.value)}
            className="h-8 border border-input bg-field/35 px-1.5 font-mono text-[11px]"
          >
            <option value="">{t("db.allTables")}</option>
            {(tableNames.length ? tableNames : triggerTables).map((tb) => (
              <option key={tb} value={tb}>
                {tb}
              </option>
            ))}
          </select>
        </div>
        {shownTriggers.length === 0 ? (
          <p className="font-mono text-[11px] text-muted-foreground/70">{t("db.noTriggers")}</p>
        ) : (
          <ul className="space-y-1">
            {shownTriggers.map((tr) => (
              <li key={`${tr.table}.${tr.name}`} className={row}>
                <div className="flex flex-wrap items-center gap-2">
                  <span className="min-w-0 flex-1 truncate" title={`${tr.table} · ${tr.function}`}>
                    {tr.table}.{tr.name} · {tr.timing} {tr.events.join("/")} · {tr.orientation}
                  </span>
                  <span className="truncate text-muted-foreground" title={tr.function}>
                    {tr.function}
                  </span>
                  {canWrite && (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => void doToggle(tr, true)}
                    >
                      {t("db.enableTrigger")}
                    </Button>
                  )}
                  {canWrite && (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => void doToggle(tr, false)}
                    >
                      {t("db.disableTrigger")}
                    </Button>
                  )}
                  {canWrite && (
                    <ConfirmButton
                      size="sm"
                      label="×"
                      confirmLabel={t("db.dropTriggerConfirm")}
                      onConfirm={() => void doDropTrigger(tr.table, tr.name)}
                    />
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </Section>

      <Section title={t("db.sequences")} description={`${seqs.length}`}>
        {seqs.length === 0 ? (
          <p className="font-mono text-[11px] text-muted-foreground/70">{t("db.noSequences")}</p>
        ) : (
          <ul className="space-y-1">
            {seqs.map((s) => (
              <li key={s.name} className={row}>
                <div className="flex flex-wrap items-center gap-2">
                  <span className="min-w-0 flex-1 truncate" title={s.data_type}>
                    {s.name}
                  </span>
                  <span className="tnum text-muted-foreground">
                    {t("db.lastValue")}: {s.last_value ?? "—"}
                  </span>
                  <span className="tnum hidden text-muted-foreground/70 xl:inline">
                    start {s.start} · min {s.min} · max {s.max} · +{s.increment}
                  </span>
                  {canWrite && (
                    <ConfirmButton
                      size="sm"
                      label={t("db.restart")}
                      confirmLabel={t("db.restartConfirm")}
                      onConfirm={() => void doRestart(s)}
                    />
                  )}
                </div>
                {canWrite && (
                  <div className="mt-1.5 flex flex-col gap-1.5 sm:flex-row sm:items-center">
                    <Input
                      value={restartWith[s.name] ?? ""}
                      onChange={(e) =>
                        setRestartWith((p) => ({ ...p, [s.name]: e.target.value }))
                      }
                      placeholder={t("db.restartWith")}
                      aria-label={t("db.restartWith")}
                      className="h-8 w-36 font-mono text-[11px]"
                    />
                    <Input
                      value={setvalIn[s.name] ?? ""}
                      onChange={(e) =>
                        setSetvalIn((p) => ({ ...p, [s.name]: e.target.value }))
                      }
                      placeholder={t("db.setval")}
                      aria-label={t("db.setval")}
                      className="h-8 w-36 font-mono text-[11px]"
                    />
                    <ConfirmButton
                      size="sm"
                      label={t("db.setval")}
                      confirmLabel={t("db.setvalConfirm")}
                      disabled={!(setvalIn[s.name] ?? "").trim()}
                      onConfirm={() => void doSetval(s)}
                    />
                  </div>
                )}
              </li>
            ))}
          </ul>
        )}
      </Section>
    </div>
  );
}
