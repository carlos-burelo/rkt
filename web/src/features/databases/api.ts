import { api } from "../../shared/lib/http";

export interface PgApp {
  app: string;
  schema: string;
  role: string;
  mode: string;
  url: string;
}

export interface PgTableDetail {
  name: string;
  rows: number;
}

export interface PgList {
  enabled: boolean;
  host?: string;
  port?: number;
  database?: string;
  apps: PgApp[];
}

export interface PgQueryResult {
  columns?: string[];
  types?: string[];
  rows?: unknown[][];
  affected?: number;
  command: string;
}

export interface PgColumnDetail {
  name: string;
  type: string;
  nullable: boolean;
  default?: string;
  is_pk: boolean;
}

export interface PgTableSchema {
  table: string;
  columns: PgColumnDetail[];
  pk: string[];
  rls: boolean;
  rls_force: boolean;
  rows: number;
}

export interface PgPolicy {
  name: string;
  cmd: string;
  roles: string;
  using?: string;
  check?: string;
}

export interface RowFilter {
  col: string;
  op: string;
  value: string;
}

export const FILTER_OPS = [
  "eq",
  "neq",
  "gt",
  "gte",
  "lt",
  "lte",
  "like",
  "ilike",
  "in",
  "isnull",
  "isnotnull",
] as const;

export function filterToString(f: RowFilter): string {
  return `${f.col}:${f.op}:${f.value}`;
}

export interface RowsOpts {
  n?: number;
  page?: number;
  order?: string;
  dir?: string;
  filters?: string[];
  search?: string;
}

export interface RowsResult {
  columns: string[];
  types: { name: string; type: string }[];
  rows: unknown[][];
  total: number;
  page: number;
}

export async function getPg(): Promise<PgList> {
  return api<PgList>("/api/v1/pg");
}

export async function provisionPg(app: string, mode: string): Promise<PgApp> {
  return api<PgApp>("/api/v1/pg", {
    method: "POST",
    body: JSON.stringify({ app, mode }),
  });
}

export async function dropPg(app: string): Promise<void> {
  await api(`/api/v1/pg/${app}`, { method: "DELETE" });
}

export async function backupPg(app: string): Promise<{ path: string }> {
  return api<{ path: string }>(`/api/v1/pg/${app}/backup`, { method: "POST" });
}

export async function restorePg(app: string, file: string): Promise<void> {
  await api("/api/v1/pg/restore", {
    method: "POST",
    body: JSON.stringify({ app, file }),
  });
}

export async function listTables(app: string): Promise<PgTableDetail[]> {
  const res = await api<{ tables: string[]; detail?: PgTableDetail[] }>(
    `/api/v1/pg/tables?app=${encodeURIComponent(app)}`,
  );
  if (res.detail) return res.detail;
  return (res.tables ?? []).map((name) => ({ name, rows: -1 }));
}

export async function getRows(
  app: string,
  table: string,
  page: number,
  n = 100,
  opts?: RowsOpts,
): Promise<RowsResult> {
  const p = new URLSearchParams({
    app,
    table,
    n: String(opts?.n ?? n),
    page: String(opts?.page ?? page),
  });
  if (opts?.order) p.set("order", opts.order);
  if (opts?.dir) p.set("dir", opts.dir);
  for (const f of opts?.filters ?? []) p.append("filter", f);
  if (opts?.search?.trim()) p.set("search", opts.search.trim());
  return api(`/api/v1/pg/rows?${p.toString()}`);
}

export async function insertRow(
  app: string,
  table: string,
  data: Record<string, unknown>,
): Promise<{ row: Record<string, unknown> }> {
  return api(`/api/v1/pg/rows`, {
    method: "POST",
    body: JSON.stringify({ app, table, data }),
  });
}

export async function updateRow(
  app: string,
  table: string,
  pk: Record<string, unknown>,
  patch: Record<string, unknown>,
): Promise<{ updated: number }> {
  return api(`/api/v1/pg/rows`, {
    method: "PATCH",
    body: JSON.stringify({ app, table, pk, patch }),
  });
}

export async function deleteRows(
  app: string,
  table: string,
  sel: { pk?: Record<string, unknown>; pks?: Record<string, unknown>[]; filters?: string[] },
): Promise<{ deleted: number }> {
  return api(`/api/v1/pg/rows`, {
    method: "DELETE",
    body: JSON.stringify({ app, table, ...sel }),
  });
}

export async function describeTable(
  app: string,
  table: string,
): Promise<{ schema: PgTableSchema; policies: PgPolicy[] }> {
  const p = new URLSearchParams({ app, table });
  return api(`/api/v1/pg/schema?${p.toString()}`);
}

export async function listSchemas(app: string): Promise<PgTableSchema[]> {
  const res = await api<{ tables: PgTableSchema[] }>(
    `/api/v1/pg/schema?app=${encodeURIComponent(app)}`,
  );
  return res.tables ?? [];
}

export interface ColumnDef {
  name: string;
  type: string;
  not_null?: boolean;
  default?: string;
  pk?: boolean;
}

export const COLUMN_TYPES = [
  "text",
  "varchar(255)",
  "integer",
  "bigint",
  "smallint",
  "boolean",
  "numeric",
  "real",
  "double precision",
  "uuid",
  "jsonb",
  "json",
  "bytea",
  "timestamptz",
  "timestamp",
  "date",
  "time",
  "inet",
] as const;

export async function createTable(app: string, table: string, columns: ColumnDef[]): Promise<void> {
  await api(`/api/v1/pg/schema`, {
    method: "POST",
    body: JSON.stringify({ app, table, columns }),
  });
}

export async function dropTable(app: string, table: string): Promise<void> {
  const p = new URLSearchParams({ app, table });
  await api(`/api/v1/pg/schema?${p.toString()}`, { method: "DELETE" });
}

export async function addColumn(app: string, table: string, column: ColumnDef): Promise<void> {
  await api(`/api/v1/pg/schema/column`, {
    method: "POST",
    body: JSON.stringify({ app, table, column }),
  });
}

export async function dropColumn(app: string, table: string, column: string): Promise<void> {
  const p = new URLSearchParams({ app, table, column });
  await api(`/api/v1/pg/schema/column?${p.toString()}`, { method: "DELETE" });
}

export async function setRLS(app: string, table: string, enable: boolean): Promise<void> {
  await api(`/api/v1/pg/schema/rls`, {
    method: "POST",
    body: JSON.stringify({ app, table, enable }),
  });
}

export async function runQuery(app: string, sql: string): Promise<PgQueryResult> {
  return api<PgQueryResult>("/api/v1/pg/query", {
    method: "POST",
    body: JSON.stringify({ app, sql }),
  });
}

export interface IndexInfo {
  name: string;
  table: string;
  def: string;
  unique: boolean;
  primary: boolean;
}

export interface ConstraintInfo {
  name: string;
  table: string;
  kind: string;
  def: string;
}

export interface BackupInfo {
  name: string;
  path: string;
  size: number;
  mtime: string;
}

// BatchResult reusa el shape de consola: cada sentencia devuelve su tabla o affected.
export type BatchResult = PgQueryResult;

export interface ExplainResult {
  columns: string[];
  rows: unknown[][];
  json: boolean;
}

export async function listIndexes(app: string, table: string): Promise<IndexInfo[]> {
  const p = new URLSearchParams({ app, table });
  const res = await api<{ indexes: IndexInfo[] }>(`/api/v1/pg/schema/indexes?${p.toString()}`);
  return res.indexes ?? [];
}

export async function createIndex(
  app: string,
  table: string,
  opts: { name?: string; columns: string[]; unique?: boolean; method?: string },
): Promise<void> {
  await api(`/api/v1/pg/schema/index`, {
    method: "POST",
    body: JSON.stringify({ app, table, ...opts }),
  });
}

export async function dropIndex(app: string, index: string): Promise<void> {
  const p = new URLSearchParams({ app, index });
  await api(`/api/v1/pg/schema/index?${p.toString()}`, { method: "DELETE" });
}

export async function listConstraints(app: string, table: string): Promise<ConstraintInfo[]> {
  const p = new URLSearchParams({ app, table });
  const res = await api<{ constraints: ConstraintInfo[] }>(
    `/api/v1/pg/schema/constraints?${p.toString()}`,
  );
  return res.constraints ?? [];
}

export async function createPolicy(
  app: string,
  table: string,
  policy: { name: string; cmd?: string; roles?: string[]; using?: string; check?: string },
): Promise<void> {
  await api(`/api/v1/pg/schema/policy`, {
    method: "POST",
    body: JSON.stringify({ app, table, ...policy }),
  });
}

export async function dropPolicy(app: string, table: string, name: string): Promise<void> {
  const p = new URLSearchParams({ app, table, name });
  await api(`/api/v1/pg/schema/policy?${p.toString()}`, { method: "DELETE" });
}

export async function renameTable(app: string, table: string, newTable: string): Promise<void> {
  await api(`/api/v1/pg/schema/rename`, {
    method: "POST",
    body: JSON.stringify({ app, table, new_table: newTable }),
  });
}

export async function renameColumn(
  app: string,
  table: string,
  column: string,
  newColumn: string,
): Promise<void> {
  await api(`/api/v1/pg/schema/column/rename`, {
    method: "POST",
    body: JSON.stringify({ app, table, column, new_column: newColumn }),
  });
}

export interface AlterColumnOpts {
  type?: string | null;
  default?: string | null;
  drop_default?: boolean;
  not_null?: boolean | null;
}

export async function alterColumn(
  app: string,
  table: string,
  column: string,
  opts: AlterColumnOpts,
): Promise<void> {
  await api(`/api/v1/pg/schema/column/alter`, {
    method: "POST",
    body: JSON.stringify({ app, table, column, ...opts }),
  });
}

export async function truncateTable(app: string, table: string): Promise<void> {
  await api(`/api/v1/pg/schema/truncate`, {
    method: "POST",
    body: JSON.stringify({ app, table }),
  });
}

export async function duplicateTable(
  app: string,
  table: string,
  newTable: string,
  withData = true,
): Promise<void> {
  await api(`/api/v1/pg/schema/duplicate`, {
    method: "POST",
    body: JSON.stringify({ app, table, new_table: newTable, with_data: withData }),
  });
}

export async function listBackups(app: string): Promise<BackupInfo[]> {
  const p = new URLSearchParams({ app });
  const res = await api<{ backups: BackupInfo[] }>(`/api/v1/pg/backups?${p.toString()}`);
  return res.backups ?? [];
}

export async function deleteBackup(app: string, name: string): Promise<void> {
  const p = new URLSearchParams({ app, name });
  await api(`/api/v1/pg/backups?${p.toString()}`, { method: "DELETE" });
}

export function backupDownloadUrl(app: string, name: string): string {
  const p = new URLSearchParams({ app, name });
  return `/api/v1/pg/backups/download?${p.toString()}`;
}

export interface ExportRowsOpts {
  filters?: string[];
  search?: string;
  order?: string;
  dir?: string;
}

export function exportRowsUrl(app: string, table: string, opts?: ExportRowsOpts): string {
  const p = new URLSearchParams({ app, table });
  if (opts?.order) p.set("order", opts.order);
  if (opts?.dir) p.set("dir", opts.dir);
  for (const f of opts?.filters ?? []) p.append("filter", f);
  if (opts?.search?.trim()) p.set("search", opts.search.trim());
  return `/api/v1/pg/rows/export?${p.toString()}`;
}

export async function importRows(
  app: string,
  table: string,
  columns: string[],
  rows: unknown[][],
): Promise<{ inserted: number }> {
  return api<{ inserted: number }>(`/api/v1/pg/rows/import`, {
    method: "POST",
    body: JSON.stringify({ app, table, columns, rows }),
  });
}

export async function runBatch(app: string, sql: string): Promise<{ results: BatchResult[]; count: number }> {
  return api<{ results: BatchResult[]; count: number }>("/api/v1/pg/query/batch", {
    method: "POST",
    body: JSON.stringify({ app, sql }),
  });
}

export async function explainQuery(app: string, sql: string): Promise<ExplainResult> {
  return api<ExplainResult>("/api/v1/pg/query/explain", {
    method: "POST",
    body: JSON.stringify({ app, sql }),
  });
}

// Objetos de esquema (views, funciones, procedimientos, triggers,
// secuencias). Mismos patrones que listIndexes/listBackups: GET con
// URLSearchParams y POST/DELETE con JSON o query.
export interface ViewInfo {
  name: string;
  kind: "view" | "materialized";
  def?: string;
  has_data?: boolean;
}

export interface FuncInfo {
  name: string;
  identity: string;
  args: string;
  return: string;
  lang: string;
  kind: string;
  def?: string;
}

export interface TriggerInfo {
  name: string;
  table: string;
  timing: string;
  events: string[];
  orientation: string;
  function: string;
}

export interface SeqInfo {
  name: string;
  data_type: string;
  start: number;
  min: number;
  max: number;
  increment: number;
  last_value?: number;
}

export async function listViews(app: string): Promise<ViewInfo[]> {
  const p = new URLSearchParams({ app });
  const res = await api<{ views: ViewInfo[] }>(`/api/v1/pg/schema/views?${p.toString()}`);
  return res.views ?? [];
}

export async function getViewDef(app: string, name: string): Promise<ViewInfo> {
  const p = new URLSearchParams({ app, name });
  const res = await api<{ view: ViewInfo }>(`/api/v1/pg/schema/views?${p.toString()}`);
  return res.view;
}

export async function createView(
  app: string,
  name: string,
  sql: string,
  opts?: { materialized?: boolean; or_replace?: boolean; with_data?: boolean },
): Promise<void> {
  await api(`/api/v1/pg/schema/view`, {
    method: "POST",
    body: JSON.stringify({ app, name, sql, ...opts }),
  });
}

export async function dropView(app: string, name: string): Promise<void> {
  const p = new URLSearchParams({ app, name });
  await api(`/api/v1/pg/schema/view?${p.toString()}`, { method: "DELETE" });
}

export async function refreshMatview(app: string, name: string, withData = true): Promise<void> {
  await api(`/api/v1/pg/schema/matview/refresh`, {
    method: "POST",
    body: JSON.stringify({ app, name, with_data: withData }),
  });
}

export async function listFunctions(app: string): Promise<FuncInfo[]> {
  const p = new URLSearchParams({ app });
  const res = await api<{ functions: FuncInfo[] }>(`/api/v1/pg/schema/functions?${p.toString()}`);
  return res.functions ?? [];
}

export async function getFunctionDef(app: string, func: string): Promise<FuncInfo> {
  const p = new URLSearchParams({ app, func });
  const res = await api<{ function: FuncInfo }>(`/api/v1/pg/schema/functions?${p.toString()}`);
  return res.function;
}

export async function dropFunction(app: string, func: string): Promise<void> {
  const p = new URLSearchParams({ app, func });
  await api(`/api/v1/pg/schema/function?${p.toString()}`, { method: "DELETE" });
}

export async function listProcedures(app: string): Promise<FuncInfo[]> {
  const p = new URLSearchParams({ app });
  const res = await api<{ procedures: FuncInfo[] }>(
    `/api/v1/pg/schema/procedures?${p.toString()}`,
  );
  return res.procedures ?? [];
}

export async function getProcedureDef(app: string, procedure: string): Promise<FuncInfo> {
  const p = new URLSearchParams({ app, procedure });
  const res = await api<{ procedure: FuncInfo }>(`/api/v1/pg/schema/procedures?${p.toString()}`);
  return res.procedure;
}

export async function callProcedure(app: string, procedure: string): Promise<{ result: string }> {
  return api<{ result: string }>(`/api/v1/pg/schema/procedure`, {
    method: "POST",
    body: JSON.stringify({ app, procedure }),
  });
}

export async function dropProcedure(app: string, procedure: string): Promise<void> {
  const p = new URLSearchParams({ app, procedure });
  await api(`/api/v1/pg/schema/procedure?${p.toString()}`, { method: "DELETE" });
}

export async function listTriggers(app: string, table?: string): Promise<TriggerInfo[]> {
  const p = new URLSearchParams({ app });
  if (table) p.set("table", table);
  const res = await api<{ triggers: TriggerInfo[] }>(`/api/v1/pg/schema/triggers?${p.toString()}`);
  return res.triggers ?? [];
}

export async function createTrigger(
  app: string,
  table: string,
  opts: {
    name: string;
    timing: string;
    events: string[];
    orientation?: string;
    function: string;
    condition?: string;
  },
): Promise<void> {
  await api(`/api/v1/pg/schema/trigger`, {
    method: "POST",
    body: JSON.stringify({ app, table, ...opts }),
  });
}

export async function dropTrigger(app: string, table: string, name: string): Promise<void> {
  const p = new URLSearchParams({ app, table, name });
  await api(`/api/v1/pg/schema/trigger?${p.toString()}`, { method: "DELETE" });
}

export async function toggleTrigger(
  app: string,
  table: string,
  name: string,
  enable: boolean,
): Promise<void> {
  await api(`/api/v1/pg/schema/trigger/toggle`, {
    method: "POST",
    body: JSON.stringify({ app, table, name, enable }),
  });
}

export async function listSequences(app: string): Promise<SeqInfo[]> {
  const p = new URLSearchParams({ app });
  const res = await api<{ sequences: SeqInfo[] }>(`/api/v1/pg/schema/sequences?${p.toString()}`);
  return res.sequences ?? [];
}

export async function restartSequence(app: string, name: string, withValue?: string): Promise<void> {
  await api(`/api/v1/pg/schema/sequence/restart`, {
    method: "POST",
    body: JSON.stringify(withValue ? { app, name, with: withValue } : { app, name }),
  });
}

export async function setvalSequence(
  app: string,
  name: string,
  value: number,
  isCalled = true,
): Promise<void> {
  await api(`/api/v1/pg/schema/sequence/setval`, {
    method: "POST",
    body: JSON.stringify({ app, name, value, is_called: isCalled }),
  });
}
// toCsv vuelca el resultado visible a CSV (comillas y NULL vacíos).
export function toCsv(columns: string[], rows: unknown[][]): string {
  const esc = (v: unknown): string => {
    if (v === null || v === undefined) return "";
    const s = String(v);
    return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
  };
  return [columns.map(esc).join(","), ...rows.map((r) => r.map(esc).join(","))].join("\n");
}
