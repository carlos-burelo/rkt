import { api } from "../../shared/lib/http";
import { Route } from "../../shared/lib/types";

export async function listRoutes(): Promise<Route[]> {
  return api<Route[]>("/api/v1/routes");
}

export async function addRoute(hosts: string[], target: string): Promise<void> {
  await api("/api/v1/routes", { method: "POST", body: JSON.stringify({ hosts, target }) });
}

export async function delRoute(host: string): Promise<void> {
  await api(`/api/v1/routes?host=${encodeURIComponent(host)}`, { method: "DELETE" });
}

export async function putRoute(host: string, target: string): Promise<void> {
  await api("/api/v1/routes", { method: "PUT", body: JSON.stringify({ host, target }) });
}

export async function tlsStatus(host: string): Promise<{ managed: boolean; expires: string }> {
  return api<{ managed: boolean; expires: string }>(`/api/v1/domains/${encodeURIComponent(host)}/tls`);
}

export interface DomainSuggestion {
  host: string;
  kind: "local" | "test" | "base";
  note: string;
}

// suggestDomains: dominios libres para un nombre (localhost/test/base).
export async function suggestDomains(name: string): Promise<DomainSuggestion[]> {
  const res = await api<{ suggestions: DomainSuggestion[] }>(
    `/api/v1/domains/suggest?name=${encodeURIComponent(name)}`,
  );
  return res.suggestions ?? [];
}

export interface ZoneHost {
  host: string;
  target: string;
  is_base: boolean;
}

export interface ZoneGroup {
  base: string;
  suffix: string;
  target: string;
  hosts: ZoneHost[];
}

export interface Zones {
  suffixes: string[];
  groups: ZoneGroup[];
}

// listZones: rutas locales agrupadas (apex + subdominios).
export function listZones(): Promise<Zones> {
  return api<Zones>("/api/v1/domains/zones");
}

// fixHost añade el host al archivo hosts del sistema (requiere admin).
export async function fixHost(host: string): Promise<{ ok: boolean; error?: string; hint?: string }> {
  const res = await fetch("/api/v1/domains/hosts", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ host }),
  });
  const body = (await res.json().catch(() => ({}))) as { error?: string; hint?: string };
  if (!res.ok) return { ok: false, error: body.error ?? `http ${res.status}`, hint: body.hint };
  return { ok: true };
}
