import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { PageHeader, Section, EmptyState } from "../../shared/components/patterns";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { FormDialog } from "../../shared/components/FormDialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import {
  dropPg,
  backupPg,
  restorePg,
  listBackups,
  deleteBackup,
  backupDownloadUrl,
  exportRowsUrl,
  importRows,
  getRows,
  runQuery,
  runBatch,
  explainQuery,
  insertRow,
  updateRow,
  deleteRows,
  toCsv,
  filterToString,
  FILTER_OPS,
  type PgQueryResult,
  type BatchResult,
  type ExplainResult,
  type BackupInfo,
  type RowFilter,
} from "./api";
import { SqlEditor } from "./SqlEditor";
import { RowFields } from "./RowFields";
import { SchemaView } from "./SchemaView";
import { ObjectsView } from "./ObjectsView";
import { ProvisionDialog } from "./TableDialogs";
import { useDb } from "./store";
import { isReadOnly, TEMPLATES } from "./sql";
import {
  loadSaved, saveQuery, deleteSaved,
  loadHist, pushHist, type SavedQuery,
} from "./snippets";

// parseCsv vuelca un CSV a matriz (comillas dobles "" escapadas, NULL no).
function parseCsv(text: string): string[][] {
  const out: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let q = false;
  const push = () => {
    row.push(cell);
    cell = "";
  };
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (q) {
      if (c === '"') {
        if (text[i + 1] === '"') {
          cell += '"';
          i++;
        } else {
          q = false;
        }
      } else {
        cell += c;
      }
    } else if (c === '"') {
      q = true;
    } else if (c === ",") {
      push();
    } else if (c === "\n") {
      push();
      out.push(row);
      row = [];
    } else if (c === "\r") {
      // ignora: el \n cierra la fila
    } else {
      cell += c;
    }
  }
  push();
  if (row.length > 1 || row[0] !== "") out.push(row);
  return out.filter((r) => r.some((v) => v !== ""));
}

function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

// ResultTable tabla de resultado mono 11px compartida por single, lote y plan.
function ResultTable({ res }: { res: PgQueryResult | BatchResult }) {
  if (res.command !== "SELECT" || !res.columns) {
    return null;
  }
  return (
    <div className="max-h-[380px] overflow-auto border border-border">
      <table className="w-full text-left font-mono text-[11px]">
        <thead className="sticky top-0">
          <tr className="border-b border-border bg-secondary/40">
            {(res.columns ?? []).map((c, i) => (
              <th key={`${c}-${i}`} className="px-2 py-1.5 font-medium">
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
  );
}

// Studio estilo HeidiSQL/Supabase: el explorador vive en el sidebar
// contextual (DbSidebar, mismo mecanismo que AppSidebar); aquí solo el
// contenido: datos, esquema, SQL y copias. Crear/editar va en diálogo.
export function Databases() {
  const { t, tn } = useI18n();
  const fail = useUI((s) => s.fail);
  const canWrite = useAuth((a) => a.canWrite);
  const apps = useDb((s) => s.apps);
  const enabled = useDb((s) => s.enabled);
  const sel = useDb((s) => s.sel);
  const tables = useDb((s) => s.tables);
  const table = useDb((s) => s.table);
  const schema = useDb((s) => s.schema);
  const policies = useDb((s) => s.policies);
  const tab = useDb((s) => s.tab);
  const loadApps = useDb((s) => s.loadApps);
  const refreshTables = useDb((s) => s.refreshTables);
  const loadSchema = useDb((s) => s.loadSchema);
  const [cols, setCols] = useState<string[]>([]);
  const [colTypes, setColTypes] = useState<{ name: string; type: string }[]>([]);
  const [rows, setRows] = useState<unknown[][]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(100);
  const [order, setOrder] = useState("");
  const [dir, setDir] = useState("");
  const [filters, setFilters] = useState<RowFilter[]>([]);
  const [search, setSearch] = useState("");
  const [appliedSearch, setAppliedSearch] = useState("");
  const [newFCol, setNewFCol] = useState("");
  const [newFOp, setNewFOp] = useState<string>("eq");
  const [newFVal, setNewFVal] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [editing, setEditing] = useState<{ mode: "insert" | "edit"; initial: Record<string, unknown>; key: string } | null>(null);
  const [rowDraft, setRowDraft] = useState<{ data: Record<string, unknown>; changed: boolean }>({ data: {}, changed: false });
  const [saving, setSaving] = useState(false);
  const [sql, setSql] = useState("");
  const [res, setRes] = useState<PgQueryResult | null>(null);
  const [batch, setBatch] = useState<BatchResult[] | null>(null);
  const [plan, setPlan] = useState<ExplainResult | null>(null);
  const [running, setRunning] = useState(false);
  const [elapsedMs, setElapsedMs] = useState<number | null>(null);
  const [backupPath, setBackupPath] = useState("");
  const [backups, setBackups] = useState<BackupInfo[]>([]);
  const [restoreFile, setRestoreFile] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);
  const [saved, setSaved] = useState<SavedQuery[]>([]);
  const [hist, setHist] = useState<{ sql: string; ts: number }[]>([]);
  const [saveName, setSaveName] = useState("");

  const readonly = useMemo(() => isReadOnly(sql), [sql]);
  const filtersKey = useMemo(() => filters.map(filterToString).join("\n"), [filters]);
  const pkCols = useMemo(() => schema?.pk ?? [], [schema]);

  useEffect(() => {
    void loadApps();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const cur = apps.find((a) => a.app === sel) ?? null;

  // Debounce de búsqueda (Supabase: 300ms).
  useEffect(() => {
    const id = setTimeout(() => {
      setAppliedSearch(search.trim());
      setPage(1);
    }, 350);
    return () => clearTimeout(id);
  }, [search]);

  useEffect(() => {
    if (!sel) return;
    setCols([]);
    setRows([]);
    setRes(null);
    setBatch(null);
    setPlan(null);
    setBackups([]);
    setSql("");
    setElapsedMs(null);
    setFilters([]);
    setSearch("");
    setAppliedSearch("");
    setOrder("");
    setDir("");
    setPage(1);
    setSelected(new Set());
    setEditing(null);
    setSaved(loadSaved(sel));
    setHist(loadHist(sel));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sel]);

  useEffect(() => {
    setPage(1);
    setSelected(new Set());
    setEditing(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [table]);

  useEffect(() => {
    if (!newFCol && schema && schema.columns.length) setNewFCol(schema.columns[0].name);
  }, [schema, newFCol]);

  useEffect(() => {
    if (!sel || !table || tab !== "tablas") return;
    getRows(sel, table, page, pageSize, {
      order: order || undefined,
      dir: dir || undefined,
      filters: filtersKey ? filtersKey.split("\n") : [],
      search: appliedSearch || undefined,
    })
      .then((r) => {
        setCols(r.columns);
        setColTypes(r.types ?? []);
        setRows(r.rows);
        setTotal(r.total);
        setSelected(new Set());
      })
      .catch((e) => fail(e));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sel, table, page, pageSize, order, dir, filtersKey, appliedSearch, tab]);

  const typeOf = (c: string) => colTypes.find((x) => x.name === c)?.type ?? "";
  const isPk = (c: string) => pkCols.includes(c);
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  const loadBackups = async () => {
    if (!sel) return;
    try {
      setBackups(await listBackups(sel));
    } catch (e) {
      fail(e);
    }
  };

  useEffect(() => {
    if (tab === "copias" && sel) void loadBackups();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab, sel]);

  const exportHref =
    table && sel
      ? exportRowsUrl(sel, table, {
          order: order || undefined,
          dir: dir || undefined,
          filters: filtersKey ? filtersKey.split("\n") : [],
          search: appliedSearch || undefined,
        })
      : "";

  const rowKey = (r: unknown[], i: number): string => {
    if (pkCols.length) {
      return pkCols.map((c) => String(r[cols.indexOf(c)] ?? "")).join("‖");
    }
    return `idx:${i}`;
  };

  const pkOfRow = (r: unknown[]): Record<string, unknown> | null => {
    if (!pkCols.length) return null;
    const pk: Record<string, unknown> = {};
    for (const c of pkCols) pk[c] = r[cols.indexOf(c)];
    return pk;
  };

  const toggleSort = (c: string) => {
    if (order !== c) {
      setOrder(c);
      setDir("asc");
    } else if (dir === "asc") {
      setDir("desc");
    } else {
      setOrder("");
      setDir("");
    }
    setPage(1);
  };

  const addFilter = () => {
    if (!newFCol) return;
    if (newFOp !== "isnull" && newFOp !== "isnotnull" && !newFVal.trim() && newFOp !== "in") return;
    setFilters((p) => [...p, { col: newFCol, op: newFOp, value: newFVal.trim() }]);
    setNewFVal("");
    setPage(1);
  };

  const doExportCsv = () => {
    if (!rows.length) return;
    const blob = new Blob([toCsv(cols, rows)], { type: "text/csv;charset=utf-8" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `${table}-p${page}.csv`;
    a.click();
    URL.revokeObjectURL(a.href);
  };

  const closeEditing = () => setEditing(null);

  const submitRow = async () => {
    if (!cur || !editing || !rowDraft.changed || saving) return;
    setSaving(true);
    try {
      if (editing.mode === "insert") {
        await insertRow(cur.app, table, rowDraft.data);
        toast.success(t("db.rowInserted"));
      } else {
        const patch: Record<string, unknown> = {};
        for (const c of schema?.columns ?? []) {
          const v = rowDraft.data[c.name];
          const old = editing.initial[c.name];
          if (String(v ?? "") !== String(old ?? "") || (v === null) !== (old === null)) {
            patch[c.name] = v;
          }
        }
        if (!Object.keys(patch).length) {
          toast.message(t("db.noChanges"));
          closeEditing();
          return;
        }
        const idx = rows.findIndex((r, i) => rowKey(r, i) === editing.key);
        const pk = idx >= 0 ? pkOfRow(rows[idx]) : null;
        if (!pk) return;
        const out = await updateRow(cur.app, table, pk, patch);
        if (out.updated === 0) toast.message(t("db.noChanges"));
      }
      closeEditing();
      await refreshAfterWrite();
    } catch (e) {
      fail(e);
      throw e;
    } finally {
      setSaving(false);
    }
  };

  const openEdit = (r: unknown[], i: number) => {
    const initial: Record<string, unknown> = {};
    cols.forEach((c, j) => {
      initial[c] = r[j];
    });
    setEditing({ mode: "edit", initial, key: rowKey(r, i) });
  };

  const doDeleteSelected = async () => {
    if (!cur || selected.size === 0) return;
    const pks: Record<string, unknown>[] = [];
    rows.forEach((r, i) => {
      if (!selected.has(rowKey(r, i))) return;
      const pk = pkOfRow(r);
      if (pk) pks.push(pk);
    });
    if (!pks.length) return;
    try {
      const out = await deleteRows(cur.app, table, { pks });
      toast.success(t("db.deleted", { n: out.deleted }));
      await refreshAfterWrite();
    } catch (e) {
      fail(e);
    }
  };

  const doDeleteFiltered = async () => {
    if (!cur || !filters.length) return;
    try {
      const out = await deleteRows(cur.app, table, { filters: filtersKey.split("\n") });
      toast.success(t("db.deleted", { n: out.deleted }));
      await refreshAfterWrite();
    } catch (e) {
      fail(e);
    }
  };

  const doDeleteRow = async (r: unknown[], i: number) => {
    if (!cur) return;
    const pk = pkOfRow(r);
    if (!pk) return;
    try {
      await deleteRows(cur.app, table, { pk });
      if (editing?.mode === "edit" && editing.key === rowKey(r, i)) closeEditing();
      await refreshAfterWrite();
    } catch (e) {
      fail(e);
    }
  };

  const refreshAfterWrite = async () => {
    if (!sel) return;
    await refreshTables(table);
    await loadSchema();
    try {
      const r = await getRows(sel, table, Math.min(page, Math.max(1, Math.ceil((total + 1) / pageSize))), pageSize, {
        order: order || undefined,
        dir: dir || undefined,
        filters: filtersKey ? filtersKey.split("\n") : [],
        search: appliedSearch || undefined,
      });
      setCols(r.columns);
      setColTypes(r.types ?? []);
      setRows(r.rows);
      setTotal(r.total);
      setPage(r.page);
      setSelected(new Set());
    } catch (e) {
      fail(e);
    }
  };

  const doDrop = async () => {
    if (!cur) return;
    try {
      await dropPg(cur.app);
      toast.success(t("db.dropped", { app: cur.app }));
      await loadApps("");
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
      await loadBackups();
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
      await loadBackups();
    } catch (e) {
      fail(e);
    }
  };

  const doRestorePath = async (path: string) => {
    if (!cur) return;
    try {
      await restorePg(cur.app, path);
      toast.success(t("db.restored", { file: path }));
      await loadBackups();
    } catch (e) {
      fail(e);
    }
  };

  const doDeleteBackup = async (name: string) => {
    if (!cur) return;
    try {
      await deleteBackup(cur.app, name);
      toast.success(name);
      await loadBackups();
    } catch (e) {
      fail(e);
    }
  };

  const doImportFile = async (f: File) => {
    if (!cur || !table) return;
    try {
      const grid = parseCsv(await f.text());
      if (grid.length < 2) {
        toast.message(t("db.importEmpty"));
        return;
      }
      const columns = grid[0].map((c) => c.trim()).filter(Boolean);
      const rowsIn: unknown[][] = grid
        .slice(1)
        .map((r) => columns.map((_, i) => ((r[i] ?? "") === "" ? null : r[i])));
      const out = await importRows(cur.app, table, columns, rowsIn.slice(0, 1000));
      toast.success(t("db.imported", { n: out.inserted }));
      await refreshAfterWrite();
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
      setBatch(null);
      setPlan(null);
      setElapsedMs(Math.round(performance.now() - t0));
      setHist(pushHist(cur.app, sql));
    } catch (e) {
      fail(e);
    } finally {
      setRunning(false);
    }
  };

  const doBatch = async () => {
    if (!cur || !sql.trim() || running) return;
    setRunning(true);
    const t0 = performance.now();
    try {
      const r = await runBatch(cur.app, sql);
      setBatch(r.results);
      setRes(null);
      setPlan(null);
      setElapsedMs(Math.round(performance.now() - t0));
      setHist(pushHist(cur.app, sql));
    } catch (e) {
      fail(e);
    } finally {
      setRunning(false);
    }
  };

  const doExplain = async () => {
    if (!cur || !sql.trim() || running) return;
    setRunning(true);
    const t0 = performance.now();
    try {
      const r = await explainQuery(cur.app, sql);
      setPlan(r);
      setElapsedMs(Math.round(performance.now() - t0));
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
      {apps.length === 0 ? (
        <EmptyState
          title={t("db.empty")}
          hint={t("db.emptyHint")}
          action={canWrite ? <ProvisionDialog onCreated={(app) => void loadApps(app)} /> : undefined}
        />
      ) : !cur ? (
        <EmptyState title={t("db.pickApp")} />
      ) : (
        <div className="min-w-0 space-y-4">
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

          {tab === "tablas" && (
            <Section
              title={table || t("db.tables")}
              description={tables.length ? `${tables.length} ${t("db.tablesLabel")} · ${cur.schema}` : undefined}
              actions={
                <span className="tnum font-mono text-[10px] text-muted-foreground">
                  {total} {t("db.rowsLabel")}
                </span>
              }
            >
              {tables.length === 0 || !table ? (
                <EmptyState title={t("db.noTables")} hint={t("db.sqlDesc")} />
              ) : (
                <div className="space-y-3">
                  <div className="flex flex-col gap-2 border border-border p-2 lg:flex-row lg:items-end">
                    <label className="block min-w-0 flex-1">
                      <span className="micro mb-1 block text-muted-foreground">{t("db.search")}</span>
                      <Input
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                        placeholder={t("db.searchPlaceholder")}
                        aria-label={t("db.search")}
                        className="h-8 font-mono text-[12px]"
                      />
                    </label>
                    <div className="flex flex-wrap items-end gap-1.5">
                      <label className="block">
                        <span className="micro mb-1 block text-muted-foreground">{t("db.filterCol")}</span>
                        <select
                          value={newFCol}
                          onChange={(e) => setNewFCol(e.target.value)}
                          className="h-8 border border-input bg-field/35 px-1.5 font-mono text-[11px]"
                          aria-label={t("db.filterCol")}
                        >
                          {(schema?.columns.map((c) => c.name) ?? cols).map((c) => (
                            <option key={c} value={c}>
                              {c}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label className="block">
                        <span className="micro mb-1 block text-muted-foreground">{t("db.filterOp")}</span>
                        <select
                          value={newFOp}
                          onChange={(e) => setNewFOp(e.target.value)}
                          className="h-8 border border-input bg-field/35 px-1.5 font-mono text-[11px]"
                          aria-label={t("db.filterOp")}
                        >
                          {FILTER_OPS.map((op) => (
                            <option key={op} value={op}>
                              {op}
                            </option>
                          ))}
                        </select>
                      </label>
                      {newFOp !== "isnull" && newFOp !== "isnotnull" && (
                        <label className="block">
                          <span className="micro mb-1 block text-muted-foreground">{t("db.filterVal")}</span>
                          <Input
                            value={newFVal}
                            onChange={(e) => setNewFVal(e.target.value)}
                            onKeyDown={(e) => {
                              if (e.key === "Enter") addFilter();
                            }}
                            className="h-8 w-40 font-mono text-[12px]"
                            aria-label={t("db.filterVal")}
                          />
                        </label>
                      )}
                      <Button size="sm" variant="outline" onClick={addFilter} disabled={!newFCol}>
                        + {t("db.filterAdd")}
                      </Button>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <label className="micro text-muted-foreground" htmlFor="db-pagesize">
                        n
                      </label>
                      <select
                        id="db-pagesize"
                        value={pageSize}
                        onChange={(e) => {
                          setPageSize(Number(e.target.value));
                          setPage(1);
                        }}
                        className="h-8 border border-input bg-field/35 px-1.5 font-mono text-[11px]"
                      >
                        {[25, 50, 100, 250, 500].map((n) => (
                          <option key={n} value={n}>
                            {n}
                          </option>
                        ))}
                      </select>
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={doExportCsv}
                        disabled={!rows.length}
                        title="CSV"
                      >
                        CSV
                      </Button>
                      {exportHref && (
                        <a
                          href={exportHref}
                          download={`${table}.csv`}
                          className="inline-flex h-8 items-center border border-input bg-background px-3 font-mono text-[11px] hover:bg-accent/40"
                          title={t("db.exportFull")}
                        >
                          {t("db.exportFull")}
                        </a>
                      )}
                      {canWrite && (
                        <>
                          <input
                            ref={fileRef}
                            type="file"
                            accept=".csv,text/csv,text/plain"
                            className="hidden"
                            aria-label={t("db.importCsv")}
                            onChange={(e) => {
                              const f = e.target.files?.[0];
                              e.target.value = "";
                              if (f) void doImportFile(f);
                            }}
                          />
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => fileRef.current?.click()}
                            title={t("db.importCsv")}
                          >
                            {t("db.importCsv")}
                          </Button>
                        </>
                      )}
                      {canWrite && (
                        <Button
                          size="sm"
                          onClick={() => setEditing({ mode: "insert", initial: {}, key: `new-${Date.now()}` })}
                        >
                          + {t("db.insert")}
                        </Button>
                      )}
                    </div>
                  </div>
                  {filters.length > 0 && (
                    <div className="flex flex-wrap items-center gap-1.5">
                      {filters.map((f, i) => (
                        <span
                          key={`${filterToString(f)}-${i}`}
                          className="flex items-center gap-1 border border-border bg-secondary/40 px-1.5 py-0.5 font-mono text-[11px]"
                        >
                          {f.col}:{f.op}:{f.value}
                          <button
                            type="button"
                            onClick={() => {
                              setFilters((p) => p.filter((_, j) => j !== i));
                              setPage(1);
                            }}
                            className="px-0.5 text-muted-foreground hover:text-bad"
                            aria-label={`${f.col} ${t("common.delete")}`}
                          >
                            ×
                          </button>
                        </span>
                      ))}
                      <button
                        type="button"
                        onClick={() => {
                          setFilters([]);
                          setPage(1);
                        }}
                        className="font-mono text-[11px] text-muted-foreground hover:text-foreground"
                      >
                        {t("db.filtersClear")}
                      </button>
                      {canWrite && (
                        <ConfirmButton
                          size="sm"
                          label={t("db.deleteFiltered", { n: total })}
                          confirmLabel={t("db.deleteFilteredConfirm")}
                          disabled={!filters.length}
                          onConfirm={doDeleteFiltered}
                        />
                      )}
                    </div>
                  )}

                  <div className="overflow-x-auto border border-border">
                    <table className="w-full text-left font-mono text-[11px]">
                      <thead className="sticky top-0">
                        <tr className="border-b border-border bg-secondary/40">
                          {canWrite && pkCols.length > 0 && (
                            <th className="w-9 px-2 py-1.5">
                              <Checkbox
                                checked={rows.length > 0 && selected.size === rows.length}
                                onCheckedChange={(v) => {
                                  if (v === true) setSelected(new Set(rows.map((r, i) => rowKey(r, i))));
                                  else setSelected(new Set());
                                }}
                                aria-label={t("db.selectAll")}
                              />
                            </th>
                          )}
                          {cols.map((c) => (
                            <th key={c} className="px-2 py-1.5 font-medium">
                              <button
                                type="button"
                                onClick={() => toggleSort(c)}
                                className="hover:text-foreground"
                                title={t("db.sortBy", { col: c })}
                              >
                                {c}
                                {order === c && (dir === "asc" ? " ▲" : " ▼")}
                              </button>
                              {isPk(c) && (
                                <span className="ml-1.5 border border-signal/50 px-1 text-[9px] font-normal text-signal">
                                  PK
                                </span>
                              )}
                              {typeOf(c) && (
                                <span className="ml-1.5 font-normal text-muted-foreground/70">{typeOf(c)}</span>
                              )}
                            </th>
                          ))}
                          {canWrite && <th className="w-16 px-2 py-1.5" />}
                        </tr>
                      </thead>
                      <tbody>
                        {rows.map((r, i) => {
                          const k = rowKey(r, i);
                          return (
                            <tr
                              key={k}
                              className="border-b border-border/50 last:border-0 hover:bg-accent/20"
                              onClick={() => canWrite && pkCols.length > 0 && openEdit(r, i)}
                              title={canWrite && pkCols.length > 0 ? t("db.rowEdit") : undefined}
                              style={canWrite && pkCols.length > 0 ? { cursor: "pointer" } : undefined}
                            >
                              {canWrite && pkCols.length > 0 && (
                                <td className="px-2 py-1">
                                  <span onClick={(e) => e.stopPropagation()} className="inline-flex">
                                    <Checkbox
                                      checked={selected.has(k)}
                                      onCheckedChange={(v) => {
                                        setSelected((prev) => {
                                          const next = new Set(prev);
                                          if (v === true) next.add(k);
                                          else next.delete(k);
                                          return next;
                                        });
                                      }}
                                      aria-label={`${t("db.selectAll")} ${i + 1}`}
                                    />
                                  </span>
                                </td>
                              )}
                              {r.map((v, j) => (
                                <td key={j} className="max-w-[320px] truncate px-2 py-1 text-muted-foreground" title={String(v ?? "")}>
                                  {v === null || v === undefined ? <span className="opacity-50">NULL</span> : String(v)}
                                </td>
                              ))}
                              {canWrite && (
                                <td className="px-2 py-1 text-right whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                                  {pkCols.length > 0 && (
                                    <ConfirmButton
                                      size="sm"
                                      label="×"
                                      confirmLabel={t("db.deleteRowConfirm")}
                                      onConfirm={() => doDeleteRow(r, i)}
                                    />
                                  )}
                                </td>
                              )}
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                  {rows.length === 0 && (
                    <EmptyState
                      title={filters.length || appliedSearch ? t("db.noRowsFiltered") : t("db.noRows")}
                      action={
                        canWrite ? (
                          <Button
                            size="sm"
                            onClick={() => setEditing({ mode: "insert", initial: {}, key: `new-${Date.now()}` })}
                          >
                            + {t("db.insert")}
                          </Button>
                        ) : undefined
                      }
                    />
                  )}
                  {!pkCols.length && rows.length > 0 && canWrite && (
                    <p className="font-mono text-[10px] text-warn">{t("db.noPkHint")}</p>
                  )}
                  <div className="flex flex-wrap items-center gap-2">
                    <Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
                      {t("db.prev")}
                    </Button>
                    <span className="tnum font-mono text-[10px] text-muted-foreground">
                      {t("db.page", { page })} / {totalPages} · {total}
                    </span>
                    <Button size="sm" variant="outline" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
                      {t("db.next")}
                    </Button>
                    {selected.size > 0 && (
                      <ConfirmButton
                        size="sm"
                        label={t("db.deleteSelected", { n: selected.size })}
                        confirmLabel={t("db.deleteSelectedConfirm")}
                        onConfirm={doDeleteSelected}
                      />
                    )}
                  </div>
                </div>
              )}
            </Section>
          )}

          {tab === "esquema" && (
            tables.length === 0 ? (
              <EmptyState title={t("db.noTables")} hint={t("db.createTableDesc")} />
            ) : (
              <SchemaView
                app={cur.app}
                schema={schema}
                policies={policies}
                canWrite={canWrite}
                onChanged={() => {
                  void refreshTables(table).then(() => loadSchema());
                }}
              />
            )
          )}

          {tab === "objetos" && (
            <ObjectsView
              app={cur.app}
              canWrite={canWrite}
              onChanged={() => {
                void refreshTables(table).then(() => loadSchema());
              }}
            />
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
                {canWrite && (
                  <div className="mt-2 flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={running || !sql.trim()}
                      onClick={doBatch}
                    >
                      {t("db.runBatch")}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={running || !sql.trim()}
                      onClick={doExplain}
                    >
                      {t("db.explain")}
                    </Button>
                  </div>
                )}
                {res && (
                  <div className="mt-3">
                    <p className="tnum mb-1.5 font-mono text-[10px] uppercase tracking-[0.08em] text-muted-foreground">
                      {t("db.results")}
                      {elapsedMs !== null && ` · ${elapsedMs} ms`}
                      {res.command === "SELECT" && ` · ${(res.rows ?? []).length}`}
                    </p>
                    {res.command === "SELECT" ? (
                      <ResultTable res={res} />
                    ) : (
                      <p className="tnum font-mono text-[11px] text-ok">{t("db.affected", { n: res.affected ?? 0 })}</p>
                    )}
                  </div>
                )}
                {batch && (
                  <div className="mt-3 space-y-3">
                    <p className="tnum font-mono text-[10px] uppercase tracking-[0.08em] text-muted-foreground">
                      {t("db.batchResults", { n: batch.length })}
                      {elapsedMs !== null && ` · ${elapsedMs} ms`}
                    </p>
                    {batch.map((r, i) => (
                      <div key={i}>
                        <p className="tnum mb-1 font-mono text-[10px] text-muted-foreground">
                          #{i + 1} · {r.command}
                          {r.command !== "SELECT" && ` · ${t("db.affected", { n: r.affected ?? 0 })}`}
                          {r.command === "SELECT" && ` · ${(r.rows ?? []).length}`}
                        </p>
                        <ResultTable res={r} />
                      </div>
                    ))}
                  </div>
                )}
                {plan && (
                  <div className="mt-3">
                    <p className="tnum mb-1.5 font-mono text-[10px] uppercase tracking-[0.08em] text-muted-foreground">
                      {t("db.explain")}
                      {elapsedMs !== null && ` · ${elapsedMs} ms`}
                    </p>
                    <ResultTable res={{ command: "SELECT", columns: plan.columns, rows: plan.rows }} />
                    {plan.json && (
                      <p className="mt-1 font-mono text-[10px] text-muted-foreground">{t("db.explainJson")}</p>
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
                <div>
                  <p className="tnum mb-1.5 font-mono text-[10px] uppercase tracking-[0.08em] text-muted-foreground">
                    {t("db.backupsList", { n: backups.length })}
                  </p>
                  {backups.length === 0 ? (
                    <p className="font-mono text-[11px] text-muted-foreground/70">{t("db.noBackups")}</p>
                  ) : (
                    <ul className="space-y-1">
                      {backups.map((b) => (
                        <li
                          key={b.name}
                          className="flex flex-wrap items-center gap-2 border border-border/60 bg-card/40 px-2 py-1.5 font-mono text-[11px]"
                        >
                          <span className="min-w-0 flex-1 truncate" title={b.path}>
                            {b.name}
                          </span>
                          <span className="tnum text-muted-foreground">{fmtSize(b.size)}</span>
                          <span className="tnum text-muted-foreground">{b.mtime}</span>
                          <a
                            href={cur ? backupDownloadUrl(cur.app, b.name) : "#"}
                            download={b.name}
                            className="border border-input px-1.5 py-0.5 text-muted-foreground hover:bg-accent/40 hover:text-foreground"
                          >
                            {t("db.download")}
                          </a>
                          {canWrite && (
                            <>
                              <ConfirmButton
                                size="sm"
                                label={t("db.restore")}
                                confirmLabel={t("db.restoreConfirm")}
                                onConfirm={() => doRestorePath(b.path)}
                              />
                              <ConfirmButton
                                size="sm"
                                label="×"
                                confirmLabel={t("db.deleteBackupConfirm")}
                                onConfirm={() => doDeleteBackup(b.name)}
                              />
                            </>
                          )}
                        </li>
                      ))}
                    </ul>
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

      {editing && schema && (
        <FormDialog
          trigger={<button type="button" className="hidden" aria-hidden="true" tabIndex={-1} />}
          title={editing.mode === "insert" ? t("db.rowInsertTitle") : t("db.rowEditTitle")}
          description={`${table} · PK: ${pkCols.length ? pkCols.join(", ") : "—"}`}
          submitLabel={editing.mode === "insert" ? t("db.insert") : t("common.save")}
          canSubmit={rowDraft.changed && !saving}
          onSubmit={submitRow}
          onOpenChange={(o) => {
            if (!o) closeEditing();
          }}
          className="sm:max-w-xl"
          open
        >
          <RowFields
            key={`${table}:${editing.mode}:${editing.key}`}
            columns={schema.columns}
            initial={editing.initial}
            mode={editing.mode}
            onChange={(data, changed) => setRowDraft({ data, changed })}
          />
        </FormDialog>
      )}
    </div>
  );
}
