import { api } from "../../shared/lib/http";

export async function getSettings(): Promise<Record<string, string>> {
  return api<Record<string, string>>("/api/v1/settings");
}

export async function setSetting(key: string, value: string): Promise<void> {
  await api("/api/v1/settings", { method: "PUT", body: JSON.stringify({ key, value }) });
}

export interface GithubStatus {
  configured: boolean;
  app_id?: number;
  app_slug?: string;
  account?: string;
  repository_selection?: string;
  repositories?: string | number;
  install_url?: string;
  install_target?: string;
  installations?: number;
  setup_url?: string;
  env?: string;
  fix?: string;
  local?: boolean;
  stale?: boolean;
  install_pending?: boolean;
  webhook_active?: boolean;
  warning?: string;
  error?: string;
}

export interface GithubTestResult {
  ok: boolean;
  account?: string;
  installations?: number;
  repositories?: number;
  repository_selection?: string;
  latency_ms?: number;
  error?: string;
}

export async function getGithubStatus(): Promise<GithubStatus> {
  return api<GithubStatus>("/api/v1/github");
}

export async function disconnectGithub(): Promise<void> {
  await api("/api/v1/github", { method: "DELETE" });
}

export async function testGithub(): Promise<GithubTestResult> {
  return api<GithubTestResult>("/api/v1/github/test", { method: "POST" });
}

export interface ApiToken {
  id: number;
  name: string;
  role: string;
  created_at: string;
  expires_at: string;
  last_used: string;
}

export async function listTokens(): Promise<ApiToken[]> {
  return api<ApiToken[]>("/api/v1/tokens");
}

export async function createToken(name: string, role: string, ttlHours: number): Promise<string> {
  const res = await api<{ token: string }>("/api/v1/tokens", {
    method: "POST",
    body: JSON.stringify({ name, role, ttl_hours: ttlHours }),
  });
  return res.token;
}

export async function revokeToken(id: number): Promise<void> {
  await api(`/api/v1/tokens/${id}`, { method: "DELETE" });
}

export interface GitCred {
  name: string;
  kind: "pat" | "ssh";
  match: string;
  public_key?: string;
  fingerprint?: string;
  created_at: string;
}

export interface GithubReport {
  installations: { id: number; account: string; type: string; selection: string; repos: number; settings_url?: string }[];
  missing: { account: string; type: string; install_url: string }[];
  needs_all_repos: string[];
  org_detection: boolean;
  notes: string[];
  healthy: boolean;
}

export async function listGitCreds(): Promise<GitCred[]> {
  return api<GitCred[]>("/api/v1/git/credentials");
}

export async function addGitCred(body: { name: string; kind: "pat" | "ssh"; match: string; secret: string }): Promise<{ deploy_key?: string }> {
  return api<{ deploy_key?: string }>("/api/v1/git/credentials", { method: "POST", body: JSON.stringify(body) });
}

export async function delGitCred(name: string): Promise<void> {
  await api(`/api/v1/git/credentials/${encodeURIComponent(name)}`, { method: "DELETE" });
}

export async function revalidateGithub(): Promise<GithubReport> {
  return api<GithubReport>("/api/v1/github/revalidate", { method: "POST" });
}
