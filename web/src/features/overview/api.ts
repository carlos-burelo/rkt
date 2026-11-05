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
  buckets: { ts: string; req: number; err: number; p50?: string; p95?: string; p99?: string }[];
  classes?: Record<string, number>;
  by_app?: { app: string; req: number; err: number; bytes: number; p95?: string }[];
  restarts: number;
  down: string[];
  p50?: string;
  p95?: string;
  p99?: string;
}

export async function fetchObs(hours = 6): Promise<Obs> {
  return api<Obs>(`/api/v1/observability?hours=${hours}`);
}

export interface Usage {
  days: number;
  total: number;
  bytes: number;
  err_rate: number;
  p95: string;
  by_day: Record<string, number>;
}

export async function fetchUsage(days = 30): Promise<Usage> {
  return api<Usage>(`/api/v1/usage?days=${days}`);
}
