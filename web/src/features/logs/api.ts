import { api } from "../../shared/lib/http";

export async function fetchLog(app: string, n = 200): Promise<string> {
  const q = `/api/v1/logs?n=${n}${app !== "" ? `&app=${app}` : ""}`;
  return (await api<{ log: string }>(q)).log;
}
