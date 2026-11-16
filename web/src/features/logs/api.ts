import { api } from "../../shared/lib/http";

export async function fetchLog(app: string, n = 200): Promise<string> {
  const q = `/api/v1/logs?n=${n}${app !== "" ? `&app=${app}` : ""}`;
  return (await api<{ log?: string }>(q)).log ?? "";
}

// clearLogs vacía los logs operativos (admin). Devuelve nº de archivos.
export async function clearLogs(): Promise<number> {
  return (await api<{ files?: number }>(`/api/v1/logs/clear`, { method: "POST" })).files ?? 0;
}
