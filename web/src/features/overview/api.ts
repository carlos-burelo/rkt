import { api } from "../../shared/lib/http";
import { Status, Deployment, RktEvent } from "../../shared/lib/types";

export async function fetchStatus(): Promise<Status> {
  return api<Status>("/api/v1/status");
}

export async function fetchDeployments(n = 50): Promise<Deployment[]> {
  return api<Deployment[]>(`/api/v1/deployments?n=${n}`);
}

export async function fetchEvents(n = 30): Promise<RktEvent[]> {
  return api<RktEvent[]>(`/api/v1/events?n=${n}`);
}

export interface Analytics {
  total: number;
  bytes: number;
  by_class: Record<string, number>;
  by_day: Record<string, number>;
  by_path: { k: string; v: number }[];
  by_host: { k: string; v: number }[];
  by_ref: { k: string; v: number }[];
  p50: string;
  p95: string;
}

export async function fetchAnalytics(host: string, days = 7): Promise<Analytics> {
  return api<Analytics>(`/api/v1/analytics?host=${encodeURIComponent(host)}&days=${days}`);
}

export interface Obs {
  hours: number;
  buckets: { ts: string; req: number; err: number }[];
  restarts: number;
  down: string[];
}

export async function fetchObs(hours = 6): Promise<Obs> {
  return api<Obs>(`/api/v1/observability?hours=${hours}`);
}
