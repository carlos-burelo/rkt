import { api } from "../../shared/lib/http";

export interface PgApp {
  app: string;
  schema: string;
  role: string;
  mode: string;
  url: string;
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

export async function listTables(app: string): Promise<string[]> {
  const res = await api<{ tables: string[] }>(`/api/v1/pg/tables?app=${encodeURIComponent(app)}`);
  return res.tables;
}

export async function getRows(
  app: string,
  table: string,
  page: number,
  n = 100,
): Promise<{ columns: string[]; types: { name: string; type: string }[]; rows: unknown[][]; total: number }> {
  return api(`/api/v1/pg/rows?app=${encodeURIComponent(app)}&table=${encodeURIComponent(table)}&n=${n}&page=${page}`);
}

export async function runQuery(app: string, sql: string): Promise<PgQueryResult> {
  return api<PgQueryResult>("/api/v1/pg/query", {
    method: "POST",
    body: JSON.stringify({ app, sql }),
  });
}
