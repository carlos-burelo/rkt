import { api } from "../../shared/lib/http";

export interface ServerSnapshot {
  ts: string;
  hostname: string;
  os: string;
  uptime_s: number;
  cpu: { percent: number; cores: number; per_core?: number[] };
  load: { l1: number; l5: number; l15: number; supported: boolean };
  memory: {
    total: number;
    used: number;
    available: number;
    cached?: number;
    swap_total: number;
    swap_used: number;
    percent: number;
  };
  disks: { mount: string; fs_type?: string; total: number; used: number; free: number; percent: number }[];
  network: { rx_bytes: number; tx_bytes: number; rx_rate: number; tx_rate: number };
  temps: { zone: string; celsius: number }[];
  firewall: { available: boolean; enabled: boolean; profiles?: string[]; detail?: string };
  warnings?: string[];
}

export interface ServerPoint {
  ts: string;
  cpu: number;
  mem: number;
  disk: number;
  rx_rate: number;
  tx_rate: number;
  load1: number;
  temp_max?: number;
}

export interface ServerProcess {
  pid: number;
  name: string;
  rss_mb: number;
  cpu_pct: number;
  threads?: number;
}

export function fetchServer(): Promise<ServerSnapshot> {
  return api<ServerSnapshot>("/api/v1/server");
}

export function fetchServerHistory(hours = 6): Promise<ServerPoint[]> {
  return api<ServerPoint[]>(`/api/v1/server/history?hours=${hours}`);
}

export function fetchServerProcesses(limit = 10): Promise<ServerProcess[]> {
  return api<ServerProcess[]>(`/api/v1/server/processes?limit=${limit}`);
}

export interface DoctorCheck {
  name: string;
  state: "ok" | "warn" | "fail";
  detail: string;
  hint?: string;
}

export function fetchDoctor(): Promise<DoctorCheck[]> {
  return api<DoctorCheck[]>("/api/v1/server/doctor");
}
