import { api } from "../../shared/lib/http";

export interface AlertRule {
  id: number;
  app: string;
  kind: string;
  threshold: number;
  window_min: number;
  last_fired: string;
  created_at: string;
}

export async function listRules(): Promise<AlertRule[]> {
  return api<AlertRule[]>("/api/v1/alerts");
}

export async function addRule(app: string, kind: string, threshold: number, windowMin: number): Promise<void> {
  await api("/api/v1/alerts", { method: "POST", body: JSON.stringify({ app, kind, threshold, window_min: windowMin }) });
}

export async function delRule(id: number): Promise<void> {
  await api(`/api/v1/alerts/${id}`, { method: "DELETE" });
}
