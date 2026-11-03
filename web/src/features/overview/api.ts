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
