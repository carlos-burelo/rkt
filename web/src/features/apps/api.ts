import { api } from "../../shared/lib/http";
import { AppMeta, AppUsage } from "../../shared/lib/types";

export async function listApps(): Promise<AppMeta[]> {
  return api<AppMeta[]>("/api/v1/apps");
}

export async function appAction(name: string, action: "restart" | "deploy" | "rollback" | "start" | "stop"): Promise<void> {
  await api(`/api/v1/apps/${name}/${action}`, { method: "POST" });
}

export async function getEnvKeys(app: string): Promise<string[]> {
  return api<string[]>(`/api/v1/apps/${app}/env`);
}

export async function getEnvValue(app: string, key: string): Promise<string> {
  const res = await api<{ value: string }>(`/api/v1/apps/${app}/env/${encodeURIComponent(key)}`);
  return res.value;
}

export async function getUsage(app: string, windowSec = 300): Promise<AppUsage> {
  return api<AppUsage>(`/api/v1/apps/${app}/usage?window=${windowSec}`);
}

export interface GithubRepo {
  full_name: string;
  default_branch: string;
  private: boolean;
  owner: string;
}

export async function listGithubRepos(): Promise<GithubRepo[]> {
  return api<GithubRepo[]>("/api/v1/github/repos");
}

export async function patchAppEnv(app: string, set: Record<string, string>, unset: string[] = []): Promise<void> {
  await api(`/api/v1/apps/${app}/env`, { method: "PATCH", body: JSON.stringify({ set, unset }) });
}

export async function importAppEnv(app: string, raw: string): Promise<string[]> {
  const res = await api<{ keys: string[] }>(`/api/v1/apps/${app}/env/import`, {
    method: "POST",
    body: JSON.stringify({ raw }),
  });
  return res.keys ?? [];
}

export interface AppPatch {
  command?: string;
  args?: string[];
  dir?: string;
  env_file?: string;
  static?: string;
  port?: number;
  domains?: string[];
  restart_delay?: string;
  min_uptime?: string;
  memory_mb?: number;
  disabled?: boolean;
  health_path?: string;
  deploy?: { repo: string; ref: string; build: string[]; auto: boolean } | null;
}

export async function updateApp(name: string, patch: AppPatch): Promise<void> {
  await api(`/api/v1/apps/${name}`, { method: "PUT", body: JSON.stringify(patch) });
}

export async function deleteApp(name: string): Promise<{ hint?: string }> {
  return api<{ hint?: string }>(`/api/v1/apps/${name}`, { method: "DELETE" });
}

export interface CreateAppIn {
  repo?: string;
  name?: string;
  runtime?: "managed" | "external";
  domains?: string[];
  port?: number;
  command?: string;
  dir?: string;
  auto?: boolean;
}

// DetectOut plan de build inferido (sin crear la app).
export interface DetectOut {
  path: string;
  framework: string;
  pm?: string;
  install: string[];
  build: string[];
  start: string[];
  static_dir?: string;
  static_abs?: string;
  evidence?: string[];
  node_version?: string;
  workspace?: boolean;
}

export function detectApp(path: string): Promise<DetectOut> {
  return api<DetectOut>(`/api/v1/detect?path=${encodeURIComponent(path)}`);
}

// buildApp compila una app local y devuelve el id de la salida.
export async function buildApp(name: string): Promise<number> {
  const res = await api<{ deployment: number }>(`/api/v1/apps/${name}/build`, { method: "POST" });
  return res.deployment;
}

// deployAppAsync lanza deploy git en segundo plano y devuelve su id.
export async function deployAppAsync(name: string): Promise<number> {
  const res = await api<{ deployment: number }>(`/api/v1/apps/${name}/deploy?async=1`, { method: "POST" });
  return res.deployment;
}

export async function createApp(input: CreateAppIn): Promise<string> {
  const res = await api<{ app: string }>("/api/v1/apps", { method: "POST", body: JSON.stringify(input) });
  return res.app;
}
