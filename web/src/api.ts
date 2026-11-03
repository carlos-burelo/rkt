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
