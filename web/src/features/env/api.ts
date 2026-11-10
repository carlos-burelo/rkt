import { api } from "../../shared/lib/http";

export interface EnvIndex {
  global: string[];
  apps: Record<string, string[]>;
}

// getEnvIndex solo claves: los valores se revelan por clave.
export async function getEnvIndex(): Promise<EnvIndex> {
  return api<EnvIndex>("/api/v1/env");
}

export async function getGlobalValue(key: string): Promise<string> {
  const res = await api<{ value: string }>(`/api/v1/env/global/${encodeURIComponent(key)}`);
  return res.value;
}

export async function patchGlobalEnv(set: Record<string, string>, unset: string[] = []): Promise<void> {
  await api("/api/v1/env/global", { method: "PUT", body: JSON.stringify({ set, unset }) });
}

export async function importGlobalEnv(raw: string): Promise<string[]> {
  const res = await api<{ keys: string[] }>("/api/v1/env/global/import", {
    method: "POST",
    body: JSON.stringify({ raw }),
  });
  return res.keys ?? [];
}
