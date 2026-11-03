import { api } from "../../shared/lib/http";
import { AppMeta } from "../../shared/lib/types";

export async function listApps(): Promise<AppMeta[]> {
  return api<AppMeta[]>("/api/v1/apps");
}

export async function appAction(name: string, action: "restart" | "deploy" | "rollback"): Promise<void> {
  await api(`/api/v1/apps/${name}/${action}`, { method: "POST" });
}

export async function getEnv(app: string): Promise<Record<string, string>> {
  return api<Record<string, string>>(`/api/v1/apps/${app}/env`);
}

export async function setEnv(app: string, key: string, value: string): Promise<void> {
  await api(`/api/v1/apps/${app}/env`, { method: "PUT", body: JSON.stringify({ key, value }) });
}

export async function delEnv(app: string, key: string): Promise<void> {
  await api(`/api/v1/apps/${app}/env?key=${encodeURIComponent(key)}`, { method: "DELETE" });
}

export interface AppPatch {
  command?: string;
  args?: string[];
  dir?: string;
  restart_delay?: string;
  min_uptime?: string;
  memory_mb?: number;
  deploy?: { repo: string; ref: string; build: string[]; auto: boolean } | null;
}

export async function updateApp(name: string, patch: AppPatch): Promise<void> {
  await api(`/api/v1/apps/${name}`, { method: "PUT", body: JSON.stringify(patch) });
}

export async function deleteApp(name: string): Promise<{ hint?: string }> {
  return api<{ hint?: string }>(`/api/v1/apps/${name}`, { method: "DELETE" });
}
