import { api } from "../../shared/lib/http";

export interface AlertRule {
  id: number;
  app: string;
  kind: string;
  threshold: number;
  window_min: number;
  enabled: boolean;
  channel_id: number;
  last_fired: string;
  created_at: string;
}

export interface AlertChannel {
  id: number;
  name: string;
  url: string;
  enabled: boolean;
  created_at: string;
}

export interface AlertFire {
  id: number;
  rule_id: number;
  ts: string;
  value: number;
  threshold: number;
  message: string;
  resolved_at: string;
}

export async function listRules(): Promise<AlertRule[]> {
  return api<AlertRule[]>("/api/v1/alerts");
}

export interface RuleIn {
  app: string;
  kind: string;
  threshold: number;
  window_min: number;
  enabled?: boolean;
  channel_id?: number;
}

export async function addRule(input: RuleIn): Promise<void> {
  await api("/api/v1/alerts", { method: "POST", body: JSON.stringify(input) });
}

export async function updateRule(id: number, input: RuleIn): Promise<void> {
  await api(`/api/v1/alerts/${id}`, { method: "PUT", body: JSON.stringify(input) });
}

export async function delRule(id: number): Promise<void> {
  await api(`/api/v1/alerts/${id}`, { method: "DELETE" });
}

export async function testRule(id: number): Promise<string> {
  const res = await api<{ message: string }>(`/api/v1/alerts/${id}/test`, { method: "POST" });
  return res.message;
}

export async function listFires(id: number): Promise<AlertFire[]> {
  return api<AlertFire[]>(`/api/v1/alerts/${id}/fires`);
}

export async function listChannels(): Promise<AlertChannel[]> {
  return api<AlertChannel[]>("/api/v1/alert-channels");
}

export async function addChannel(name: string, url: string): Promise<void> {
  await api("/api/v1/alert-channels", { method: "POST", body: JSON.stringify({ name, url }) });
}

export async function updateChannel(id: number, name: string, url: string, enabled: boolean): Promise<void> {
  await api(`/api/v1/alert-channels/${id}`, { method: "PUT", body: JSON.stringify({ name, url, enabled }) });
}

export async function delChannel(id: number): Promise<void> {
  await api(`/api/v1/alert-channels/${id}`, { method: "DELETE" });
}
