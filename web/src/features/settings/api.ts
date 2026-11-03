import { api } from "../../shared/lib/http";

export async function getSettings(): Promise<Record<string, string>> {
  return api<Record<string, string>>("/api/v1/settings");
}

export async function setSetting(key: string, value: string): Promise<void> {
  await api("/api/v1/settings", { method: "PUT", body: JSON.stringify({ key, value }) });
}
