import { api } from "../../shared/lib/http";

export interface CronRun {
  id: number;
  app: string;
  job: string;
  started_at: string;
  finished_at: string;
  status: string;
  exit_code: number;
  log_path: string;
  message: string;
}

export type CronType = "command" | "http";

export interface CronJob {
  app: string;
  job: string;
  schedule: string;
  type: CronType;
  command?: string;
  path?: string;
  method?: string;
  timeout?: string;
  disabled: boolean;
  running: boolean;
  next?: string;
  last?: {
    status: string;
    started_at: string;
    finished_at: string;
    exit_code: number;
    message: string;
  };
}

export interface CronIn {
  schedule: string;
  type: CronType;
  command?: string;
  path?: string;
  method?: string;
  timeout?: string;
  disabled?: boolean;
}

export function listCron(): Promise<CronJob[]> {
  return api<CronJob[]>("/api/v1/cron");
}

export function saveCron(app: string, job: string, input: CronIn): Promise<void> {
  return api(`/api/v1/cron/${encodeURIComponent(app)}/${encodeURIComponent(job)}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function deleteCron(app: string, job: string): Promise<void> {
  return api(`/api/v1/cron/${encodeURIComponent(app)}/${encodeURIComponent(job)}`, { method: "DELETE" });
}

export function runCron(app: string, job: string): Promise<number> {
  return api<{ run: number }>(`/api/v1/cron/${encodeURIComponent(app)}/${encodeURIComponent(job)}/run`, {
    method: "POST",
  }).then((r) => r.run);
}

export function listCronRuns(app: string, job: string, n = 30): Promise<CronRun[]> {
  return api<CronRun[]>(
    `/api/v1/cron/${encodeURIComponent(app)}/${encodeURIComponent(job)}/runs?n=${n}`,
  );
}

export async function cronRunLog(id: number, n = 300): Promise<string> {
  const res = await api<{ log?: string }>(`/api/v1/cron/runs/${id}/log?n=${n}`);
  return res.log ?? "";
}
