import { api } from "../../shared/lib/http";

// Variables globales: las hereda toda la flota; una app puede sobrescribirlas.
export async function getGlobalEnv(): Promise<Record<string, string>> {
  return api<Record<string, string>>("/api/v1/env/global");
}

export async function setGlobalEnv(key: string, value: string): Promise<void> {
  await api("/api/v1/env/global", { method: "PUT", body: JSON.stringify({ key, value }) });
}

export async function delGlobalEnv(key: string): Promise<void> {
  await api(`/api/v1/env/global?key=${encodeURIComponent(key)}`, { method: "DELETE" });
}
