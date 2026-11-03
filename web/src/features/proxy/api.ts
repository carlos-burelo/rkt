import { api } from "../../shared/lib/http";

export interface ProxyRoute {
  hosts: string[];
  target: string;
  ok: boolean;
  tls: string[];
  /** por host: resuelve a loopback en esta máquina */
  dns_ok?: boolean[];
}

export async function fetchProxy(): Promise<ProxyRoute[]> {
  return api<ProxyRoute[]>("/api/v1/proxy");
}
