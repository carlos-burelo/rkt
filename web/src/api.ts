// Cliente api/v1. Sesión por cookie httpOnly (credenciales en cada fetch).
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (res.status === 401) {
    throw new Error("no autorizado (login)");
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error((body as { error?: string }).error ?? `http ${res.status}`);
  }
  return (await res.json()) as T;
}

export interface Me {
  user: string;
  role: string;
}

export async function login(user: string, password: string): Promise<Me> {
  const me = await api<Me>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ user, password }),
  });
  return me;
}

export async function logout(): Promise<void> {
  try {
    await api<{ ok: string }>("/api/v1/auth/logout", { method: "POST" });
  } catch {
    // sesión ya inválida: igual se sale
  }
}

export async function me(): Promise<Me> {
  return api<Me>("/api/v1/auth/me");
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

export interface User {
  name: string;
  role: string;
  created_at: string;
}

export interface AppMeta {
  name: string;
  command: string;
  args: string[];
  dir: string;
  restart_delay: string;
  min_uptime: string;
  deploy: { repo: string; ref: string; build: string[]; auto: boolean } | null;
}

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

export interface Route {
  hosts: string[];
  target: string;
}

export async function listRoutes(): Promise<Route[]> {
  return api<Route[]>("/api/v1/routes");
}

export async function addRoute(hosts: string[], target: string): Promise<void> {
  await api("/api/v1/routes", { method: "POST", body: JSON.stringify({ hosts, target }) });
}

export async function delRoute(host: string): Promise<void> {
  await api(`/api/v1/routes?host=${encodeURIComponent(host)}`, { method: "DELETE" });
}

export async function getSettings(): Promise<Record<string, string>> {
  return api<Record<string, string>>("/api/v1/settings");
}

export async function setSetting(key: string, value: string): Promise<void> {
  await api("/api/v1/settings", { method: "PUT", body: JSON.stringify({ key, value }) });
}

export async function listUsers(): Promise<User[]> {
  return api<User[]>("/api/v1/users");
}

export async function createUser(name: string, password: string, role: string): Promise<void> {
  await api("/api/v1/users", { method: "POST", body: JSON.stringify({ name, password, role }) });
}

export async function deleteUser(name: string): Promise<void> {
  await api(`/api/v1/users/${name}`, { method: "DELETE" });
}

export async function deploymentLog(id: number): Promise<string> {
  return (await api<{ log: string }>(`/api/v1/deployments/${id}/log?n=300`)).log;
}
