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

export async function tlsStatus(host: string): Promise<{ managed: boolean; expires: string }> {
  return api<{ managed: boolean; expires: string }>(`/api/v1/domains/${encodeURIComponent(host)}/tls`);
}
