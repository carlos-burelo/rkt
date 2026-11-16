import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { PageHeader, Section, EmptyState } from "../../shared/components/patterns";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import {
  getPg, provisionPg, dropPg, backupPg, restorePg,
  listTables, getRows, runQuery, type PgApp, type PgQueryResult,
} from "./api";
import { SqlEditor } from "./SqlEditor";
import { isReadOnly, TEMPLATES } from "./sql";
import {
  loadSaved, saveQuery, deleteSaved,
  loadHist, pushHist, type SavedQuery,
} from "./snippets";

// Studio de bases: visor de tablas + SQL Editor (Supabase feel) + copias.
// Sin editor visual de esquema: las tablas las crea SQL o migraciones.
// Escritura (query no-lectura, restore, drop) siempre con confirmación.
export function Databases() {
  const { t, tn } = useI18n();
  const fail = useUI((s) => s.fail);
  const canWrite = useAuth((a) => a.canWrite);
  const [enabled, setEnabled] = useState(true);
  const [apps, setApps] = useState<PgApp[]>([]);
  const [sel, setSel] = useState<string>("");
  const [provApp, setProvApp] = useState("");
  const [provMode, setProvMode] = useState("dedicated");
  const [tab, setTab] = useState<"tablas" | "sql" | "copias">("tablas");
  const [tables, setTables] = useState<string[]>([]);
  const [table, setTable] = useState("");
  const [cols, setCols] = useState<string[]>([]);
  const [colTypes, setColTypes] = useState<{ name: string; type: string }[]>([]);
  const [rows, setRows] = useState<unknown[][]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [sql, setSql] = useState("");
  const [res, setRes] = useState<PgQueryResult | null>(null);
  const [running, setRunning] = useState(false);
  const [elapsedMs, setElapsedMs] = useState<number | null>(null);
  const [backupPath, setBackupPath] = useState("");
  const [restoreFile, setRestoreFile] = useState("");
  const [saved, setSaved] = useState<SavedQuery[]>([]);
  const [hist, setHist] = useState<{ sql: string; ts: number }[]>([]);
  const [saveName, setSaveName] = useState("");

  const readonly = useMemo(() => isReadOnly(sql), [sql]);

  const load = async (keepSel?: string) => {
    try {
      const out = await getPg();
      setEnabled(out.enabled);
      setApps(out.apps);
      const want = keepSel ?? sel;
      if (out.apps.length && !out.apps.some((a) => a.app === want)) {
        setSel(out.apps[0].app);
      } else if (!want && out.apps.length) {
        setSel(out.apps[0].app);
      }
    } catch (e) {
      fail(e);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const cur = apps.find((a) => a.app === sel) ?? null;

  useEffect(() => {
    if (!sel) return;
    setTable("");
    setCols([]);
    setRows([]);
    setRes(null);
    setSql("");
    setElapsedMs(null);
    setSaved(loadSaved(sel));
    setHist(loadHist(sel));
    listTables(sel)
      .then((ts) => {
        setTables(ts);
        if (ts.length) {
          setTable(ts[0]);
          setPage(1);
        }
      })
      .catch((e) => fail(e));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sel]);

  useEffect(() => {
    if (!sel || !table || tab !== "tablas") return;
    getRows(sel, table, page)
      .then((r) => {
        setCols(r.columns);
        setColTypes(r.types ?? []);
        setRows(r.rows);
        setTotal(r.total);
      })
      .catch((e) => fail(e));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sel, table, page, tab]);

  const typeOf = (c: string) => colTypes.find((x) => x.name === c)?.type ?? "";

  const doProvision = async () => {
    const name = provApp.trim().toLowerCase();
    if (!name) return;
    try {
      await provisionPg(name, provMode);
      setProvApp("");
      toast.success(name);
      await load(name);
      setSel(name);
    } catch (e) {
      fail(e);
    }
  };

  const doDrop = async () => {
    if (!cur) return;
    try {
      await dropPg(cur.app);
      toast.success(t("db.dropped", { app: cur.app }));
      setSel("");
      await load("");
    } catch (e) {
      fail(e);
    }
  };

  const doBackup = async () => {
    if (!cur) return;
    try {
      const out = await backupPg(cur.app);
      setBackupPath(out.path);
      toast.success(t("db.backedUp", { path: out.path }));
    } catch (e) {
      fail(e);
    }
  };

  const doRestore = async () => {
    if (!cur || !restoreFile.trim()) return;
    try {
      await restorePg(cur.app, restoreFile.trim());
      toast.success(t("db.restored", { file: restoreFile.trim() }));
      setRestoreFile("");
    } catch (e) {
      fail(e);
    }
  };

  const doQuery = async () => {
    if (!cur || !sql.trim() || running) return;
    setRunning(true);
    const t0 = performance.now();
    try {
      const r = await runQuery(cur.app, sql);
      setRes(r);
      setElapsedMs(Math.round(performance.now() - t0));
      setHist(pushHist(cur.app, sql));
    } catch (e) {
      fail(e);
    } finally {
      setRunning(false);
    }
  };

  const doSave = () => {
    if (!cur || !sql.trim()) return;
    setSaved(saveQuery(cur.app, saveName, sql));
    setSaveName("");
  };

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("db.title")}
        description={t("db.description")}
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{tn("db.count", apps.length)}</span>}
      />
      {!enabled && (
        <div className="border border-warn/40 bg-warn/5 px-3 py-2">
          <p className="font-mono text-[11px] text-warn">{t("db.disabled")}</p>
        </div>
      )}
      <div className="grid gap-4 lg:grid-cols-[280px_1fr]">
        <Section title={t("db.list")}>
          <div className="space-y-2">
            {apps.length === 0 ? (
              <EmptyState title={t("db.empty")} hint={t("db.emptyHint")} />
            ) : (
              <ul className="divide-y divide-border border border-border">
                {apps.map((a) => (
                  <li key={a.app}>
                    <button
                      type="button"
                      onClick={() => setSel(a.app)}
                      className={`flex w-full items-center justify-between gap-2 px-2.5 py-2 text-left hover:bg-accent/40 ${a.app === sel ? "bg-accent/50" : ""}`}
                    >
                      <span className="truncate font-mono text-[12px]">{a.app}</span>
                      <span className="shrink-0 font-mono text-[10px] text-muted-foreground">{a.mode}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
            {canWrite && (
              <div className="border border-border p-2">
                <p className="micro mb-2 text-muted-foreground">{t("db.provisionTitle")}</p>
                <div className="flex gap-2">
                  <Input
                    value={provApp}
                    onChange={(e) => setProvApp(e.target.value)}
                    placeholder={t("db.appPlaceholder")}
                    aria-label={t("db.appPlaceholder")}
                    className="h-8 font-mono text-[12px]"
                  />
                  <select
                    value={provMode}
                    onChange={(e) => setProvMode(e.target.value)}
                    aria-label={t("db.mode")}
                    className="h-8 shrink-0 border border-input bg-field/35 px-1.5 font-mono text-[11px]"
                  >
                    <option value="dedicated">dedicated</option>
                    <option value="shared">shared</option>
                  </select>
                </div>
                <p className="mt-1.5 font-mono text-[10px] leading-relaxed text-muted-foreground/80">{t("db.provisionDesc")}</p>
                <div className="mt-2">
                  <ConfirmButton
                    size="sm"
                    label={t("db.provision")}
                    confirmLabel={t("db.runConfirm")}
                    disabled={!provApp.trim()}
                    onConfirm={doProvision}
                  />
                </div>
              </div>
            )}
          </div>
        </Section>

        <div className="min-w-0">
          {!cur ? (
            <EmptyState title={t("db.pickApp")} />
          ) : (
            <div className="space-y-4">
              <Section
                title={cur.app}
                description={`${t("db.schema")}: ${cur.schema} · ${t("db.role")}: ${cur.role} · ${cur.mode}`}
                actions={canWrite ? (
                  <div className="flex gap-2">
                    <ConfirmButton
                      size="sm"
                      label={t("db.backup")}
                      confirmLabel={t("db.backupConfirm")}
                      onConfirm={doBackup}
                    />
                    <ConfirmButton
                      size="sm"
                      label={t("db.drop")}
                      confirmLabel={t("db.dropConfirm")}
                      onConfirm={doDrop}
                    />
                  </div>
                ) : undefined}
              >
                <p className="tnum truncate font-mono text-[11px] text-muted-foreground" title={cur.url}>
                  {cur.url}
                </p>
                {backupPath && (
                  <p className="tnum mt-1 truncate font-mono text-[11px] text-ok" title={backupPath}>
                    {t("db.backedUp", { path: backupPath })}
                  </p>
                )}
              </Section>

              <div className="flex gap-1 border-b border-border">
                {(["tablas", "sql", "copias"] as const).map((k) => (
                  <button
                    key={k}
                    type="button"
                    onClick={() => setTab(k)}
                    className={`px-3 py-1.5 font-mono text-[11px] uppercase tracking-[0.08em] ${tab === k ? "border-b-2 border-signal text-foreground" : "text-muted-foreground hover:text-foreground"}`}
                  >
                    {k === "tablas" ? t("db.tables") : k === "sql" ? t("db.sql") : t("db.backups")}
                  </button>
                ))}
              </div>

              {tab === "tablas" && (
                <Section title={t("db.tables")} description={tables.length ? `${tables.length} · ${cur.schema}` : undefined}>
                  {tables.length === 0 ? (
                    <EmptyState title={t("db.noTables")} hint={t("db.sqlDesc")} />
                  ) : (
                    <div className="space-y-3">
                      <div className="flex flex-wrap gap-1.5">
                        {tables.map((tb) => (
                          <Button
                            key={tb}
                            size="sm"
                            variant={tb === table ? "default" : "outline"}
                            onClick={() => {
                              setTable(tb);
                              setPage(1);
                            }}
                          >
                            <span className="font-mono text-[11px]">{tb}</span>
                          </Button>
                        ))}
                      </div>
                      {table && (
                        <div className="overflow-x-auto border border-border">
                          <table className="w-full text-left font-mono text-[11px]">
                            <thead className="sticky top-0">
                              <tr className="border-b border-border bg-secondary/40">
                                {cols.map((c) => (
                                  <th key={c} className="px-2 py-1.5 font-medium">
                                    {c}
                                    {typeOf(c) && (
                                      <span className="ml-1.5 font-normal text-muted-foreground/70">{typeOf(c)}</span>
                                    )}
                                  </th>
                                ))}
                              </tr>
                            </thead>
                            <tbody>
                              {rows.map((r, i) => (
                                <tr key={i} className="border-b border-border/50 last:border-0 hover:bg-accent/20">
                                  {r.map((v, j) => (
                                    <td key={j} className="max-w-[320px] truncate px-2 py-1 text-muted-foreground" title={String(v ?? "")}>
                                      {v === null || v === undefined ? <span className="opacity-50">NULL</span> : String(v)}
                                    </td>
                                  ))}
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      )}
                      <div className="flex items-center gap-2">
                        <Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
                          {t("db.prev")}
                        </Button>
                        <span className="tnum font-mono text-[10px] text-muted-foreground">
                          {t("db.page", { page })} · {total}
                        </span>
                        <Button size="sm" variant="outline" disabled={rows.length < 100} onClick={() => setPage((p) => p + 1)}>
                          {t("db.next")}
                        </Button>
                      </div>
                    </div>
                  )}
                </Section>
              )}

              {tab === "sql" && (
                <div className="grid gap-4 xl:grid-cols-[220px_1fr]">
                  <div className="space-y-4">
                    <Section title={t("db.templates")}>
                      <ul className="space-y-1">
                        {TEMPLATES.map((tpl) => (
                          <li key={tpl.name}>
                            <button
                              type="button"
                              onClick={() => setSql((prev) => (prev.trim() ? prev.replace(/\s+$/, "") + "\n\n" + tpl.sql : tpl.sql))}
                              className="w-full truncate px-2 py-1.5 text-left font-mono text-[11px] text-muted-foreground hover:bg-accent/40 hover:text-foreground"
                              title={tpl.name}
                            >
                              + {tpl.name}
                            </button>
                          </li>
                        ))}
                      </ul>
                    </Section>
                    <Section title={t("db.saved")}>
                      {saved.length === 0 ? (
                        <p className="font-mono text-[10px] text-muted-foreground/70">{t("db.noSaved")}</p>
                      ) : (
                        <ul className="space-y-1">
                          {saved.map((q) => (
                            <li key={q.id} className="group flex items-center gap-1">
                              <button
                                type="button"
                                onClick={() => setSql(q.sql)}
                                className="min-w-0 flex-1 truncate px-2 py-1.5 text-left font-mono text-[11px] text-muted-foreground hover:bg-accent/40 hover:text-foreground"
                                title={q.sql}
                              >
                                {q.name}
                              </button>
                              {canWrite && (
                                <button
                                  type="button"
                                  onClick={() => setSaved(deleteSaved(cur.app, q.id))}
                                  aria-label={q.name}
                                  className="shrink-0 px-1 font-mono text-[11px] text-muted-foreground/50 opacity-0 hover:text-bad group-hover:opacity-100"
                                >
                                  ×
                                </button>
                              )}
                            </li>
                          ))}
                        </ul>
                      )}
                      <div className="mt-2 flex gap-1.5">
                        <Input
                          value={saveName}
                          onChange={(e) => setSaveName(e.target.value)}
                          placeholder={t("db.saveName")}
                          aria-label={t("db.saveName")}
                          className="h-7 font-mono text-[11px]"
                        />
                        <Button size="sm" variant="outline" disabled={!sql.trim()} onClick={doSave}>
                          {t("db.saveQuery")}
                        </Button>
                      </div>
                    </Section>
                    {hist.length > 0 && (
                      <Section title={t("db.history")}>
                        <ul className="space-y-1">
                          {hist.slice(0, 8).map((h, i) => (
                            <li key={`${h.ts}-${i}`}>
                              <button
                                type="button"
                                onClick={() => setSql(h.sql)}
                                className="w-full truncate px-2 py-1.5 text-left font-mono text-[11px] text-muted-foreground/80 hover:bg-accent/40 hover:text-foreground"
                                title={h.sql}
                              >
                                {h.sql.split("\n")[0].slice(0, 42) || "…"}
                              </button>
                            </li>
                          ))}
                        </ul>
                      </Section>
                    )}
                  </div>
                  <Section title={t("db.sql")} description={t("db.sqlDesc")}>
                    <SqlEditor
                      value={sql}
                      onChange={setSql}
                      onRun={doQuery}
                      running={running}
                      readOnly={readonly}
                      canRun={canWrite}
                      role={cur.role}
                      elapsedMs={elapsedMs}
                    />
                    {res && (
                      <div className="mt-3">
                        <p className="tnum mb-1.5 font-mono text-[10px] uppercase tracking-[0.08em] text-muted-foreground">
                          {t("db.results")}
                          {elapsedMs !== null && ` · ${elapsedMs} ms`}
                          {res.command === "SELECT" && ` · ${(res.rows ?? []).length}`}
                        </p>
                        {res.command === "SELECT" ? (
                          <div className="max-h-[380px] overflow-auto border border-border">
                            <table className="w-full text-left font-mono text-[11px]">
                              <thead className="sticky top-0">
                                <tr className="border-b border-border bg-secondary/40">
                                  {(res.columns ?? []).map((c, i) => (
                                    <th key={c} className="px-2 py-1.5 font-medium">
                                      {c}
                                      {res.types?.[i] && (
                                        <span className="ml-1.5 font-normal text-muted-foreground/70">{res.types[i]}</span>
                                      )}
                                    </th>
                                  ))}
                                </tr>
                              </thead>
                              <tbody>
                                {(res.rows ?? []).map((r, i) => (
                                  <tr key={i} className="border-b border-border/50 last:border-0 hover:bg-accent/20">
                                    {r.map((v, j) => (
                                      <td key={j} className="max-w-[320px] truncate px-2 py-1 text-muted-foreground" title={String(v ?? "")}>
                                        {v === null || v === undefined ? <span className="opacity-50">NULL</span> : String(v)}
                                      </td>
                                    ))}
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          </div>
                        ) : (
                          <p className="tnum font-mono text-[11px] text-ok">{t("db.affected", { n: res.affected ?? 0 })}</p>
                        )}
                      </div>
                    )}
                  </Section>
                </div>
              )}

              {tab === "copias" && (
                <Section title={t("db.backups")} description={t("db.backupsDesc")}>
                  <div className="flex flex-col gap-3">
                    <div className="flex flex-wrap items-center gap-2">
                      {canWrite && (
                        <ConfirmButton
                          size="sm"
                          label={t("db.backup")}
                          confirmLabel={t("db.backupConfirm")}
                          onConfirm={doBackup}
                        />
                      )}
                      {backupPath && (
                        <span className="tnum truncate font-mono text-[11px] text-muted-foreground">{backupPath}</span>
                      )}
                    </div>
                    {canWrite && (
                      <div className="flex flex-col gap-2 border-t border-border pt-3 sm:flex-row sm:items-center">
                        <Input
                          value={restoreFile}
                          onChange={(e) => setRestoreFile(e.target.value)}
                          placeholder={t("db.filePlaceholder")}
                          aria-label={t("db.restore")}
                          className="h-8 font-mono text-[11px]"
                        />
                        <ConfirmButton
                          size="sm"
                          label={t("db.restore")}
                          confirmLabel={t("db.restoreConfirm")}
                          disabled={!restoreFile.trim()}
                          onConfirm={doRestore}
                        />
                      </div>
                    )}
                  </div>
                </Section>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
