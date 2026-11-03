import { api } from "../../shared/lib/http";

export async function deploymentLog(id: number, n = 300): Promise<string> {
  return (await api<{ log?: string }>(`/api/v1/deployments/${id}/log?n=${n}`)).log ?? "";
}

export async function retryDeployment(id: number): Promise<string> {
  return (await api<{ rev: string }>(`/api/v1/deployments/${id}/retry`, { method: "POST" })).rev;
}
