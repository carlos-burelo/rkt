// Cliente api/v1. Token en localStorage (fase 1; sesiones fase 2).
const KEY = "rkt_token";

export function getToken(): string {
  return localStorage.getItem(KEY) ?? "";
}

export function setToken(t: string) {
  localStorage.setItem(KEY, t);
}

export function clearToken() {
  localStorage.removeItem(KEY);
}

export async function api<T>(path: string): Promise<T> {
  const res = await fetch(path, {
    headers: { Authorization: `Bearer ${getToken()}` },
  });
  if (res.status === 401) {
    clearToken();
    throw new Error("no autorizado (revisa dashboard_token)");
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error((body as { error?: string }).error ?? `http ${res.status}`);
  }
  return (await res.json()) as T;
}

export interface AppStatus {
  name: string;
  running: boolean;
  restarts: number;
  failed?: boolean;
}

export interface Status {
  version: string;
  routes: number;
  apps: AppStatus[];
}

export interface Deployment {
  id: number;
  app: string;
  rev: string;
  status: string;
  started_at: string;
  finished_at: string;
}

export interface RktEvent {
  id: number;
  ts: string;
  kind: string;
  app: string;
  msg: string;
}
