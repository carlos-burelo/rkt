import { api } from "../../shared/lib/http";
import type { WafBlock } from "../../shared/lib/types";

export interface WafBan {
  ip: string;
  hits: number;
  until: string;
}

export interface WafState {
  deny_paths: string[];
  deny_methods: string[];
  blocked: WafBlock[];
  banned: WafBan[];
}

export async function fetchWaf(): Promise<WafState> {
  return api<WafState>("/api/v1/waf");
}

export async function putWaf(patch: { deny_paths?: string[]; deny_methods?: string[] }): Promise<WafState> {
  return api<WafState>("/api/v1/waf", { method: "PUT", body: JSON.stringify(patch) });
}

export async function unban(ip: string): Promise<void> {
  await api(`/api/v1/waf/bans?ip=${encodeURIComponent(ip)}`, { method: "DELETE" });
}

export interface AttackCount {
  key: string;
  count: number;
}

export interface AttackEvent {
  ts: string;
  ip: string;
  host: string;
  method: string;
  path: string;
  rule: string;
  country: string;
}

export interface AttackSummary {
  hours: number;
  total: number;
  countries: AttackCount[];
  ips: AttackCount[];
  rules: AttackCount[];
  hourly: AttackCount[];
  recent: AttackEvent[];
}

export interface AttacksState {
  summary: AttackSummary;
  geoip_ready: boolean;
}

export async function fetchAttacks(hours: number): Promise<AttacksState> {
  return api<AttacksState>(`/api/v1/security/attacks?hours=${hours}`);
}

export async function updateGeoip(): Promise<{ ok: boolean; source?: string }> {
  return api<{ ok: boolean; source?: string }>("/api/v1/security/geoip", { method: "POST" });
}
