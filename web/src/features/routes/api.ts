import { api, apiResult } from "../../shared/lib/http";
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

export interface DomainDetail {
  host: string;
  target: string;
  app?: string;
  tls: { managed: boolean; expires?: string };
  dns: {
    a: string[];
    aaaa: string[];
    cname?: string;
    resolves: boolean;
    error?: string;
  };
  expected?: string;
  dns_ok: boolean;
  checked_at: string;
}

export async function domainDetail(host: string): Promise<DomainDetail> {
  return api<DomainDetail>(`/api/v1/domains/${encodeURIComponent(host)}`);
}

export interface DomainPreflight {
  host: string;
  dns: DomainDetail["dns"];
  expected?: string;
  dns_ok: boolean;
}

export async function preflightDomain(host: string): Promise<DomainPreflight> {
  return api<DomainPreflight>(`/api/v1/domains/preflight?host=${encodeURIComponent(host)}`);
}

export interface DomainSuggestion {
  host: string;
  kind: "local" | "test" | "base";
  note: string;
}

export interface DomainSuggestResult {
  suggestions: DomainSuggestion[];
  public: boolean;
  requires_domain: boolean;
  base_domain?: string;
}

// suggestDomains: dominios libres para un nombre (local o base pública).
export async function suggestDomains(name: string): Promise<DomainSuggestResult> {
  const res = await api<DomainSuggestResult>(
    `/api/v1/domains/suggest?name=${encodeURIComponent(name)}`,
  );
  return {
    suggestions: res.suggestions ?? [],
    public: res.public ?? false,
    requires_domain: res.requires_domain ?? false,
    base_domain: res.base_domain,
  };
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
  const result = await apiResult<{ ok: string }>("/api/v1/domains/hosts", {
    method: "POST",
    body: JSON.stringify({ host }),
  });
  if (!result.ok) return { ok: false, error: result.error, hint: result.hint };
  return { ok: true };
}
