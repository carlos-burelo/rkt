import { api } from "../../shared/lib/http";
import { Deployment } from "../../shared/lib/types";

export async function deploymentLog(id: number, n = 300): Promise<string> {
  return (await api<{ log?: string }>(`/api/v1/deployments/${id}/log?n=${n}`)).log ?? "";
}

// getDeployment estado puntual (polling de creación en vivo).
export function getDeployment(id: number): Promise<Deployment> {
  return api<Deployment>(`/api/v1/deployments/${id}`);
}

export interface DiffFile {
  status: string;
  from?: string;
  path: string;
}

export async function diffDeployment(app: string, from: string, to: string): Promise<DiffFile[]> {
  const res = await api<{ files?: DiffFile[] }>(
    `/api/v1/apps/${encodeURIComponent(app)}/diff?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
  );
  return res.files ?? [];
}

export async function retryDeployment(id: number): Promise<string> {
  return (await api<{ rev: string }>(`/api/v1/deployments/${id}/retry`, { method: "POST" })).rev;
}

export async function cancelDeployment(id: number): Promise<void> {
  await api(`/api/v1/deployments/${id}/cancel`, { method: "POST" });
}
