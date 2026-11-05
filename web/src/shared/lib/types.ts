// Tipos del contrato /api/v1 (único hogar; features importan de aquí).
export interface Me {
  user: string;
  role: string;
}

export interface AppStatus {
  name: string;
  running: boolean;
  restarts: number;
  failed?: boolean;
  /** respuesta HTTP real del puerto (false = proceso vivo pero mudo) */
  http_ok?: boolean;
}

export interface Status {
  version: string;
  routes: number;
  apps: AppStatus[];
  /** puerto público HTTP del proxy (dev suele ser 8080) */
  http_port?: string;
  /** puerto público HTTPS (vacío = TLS off) */
  https_port?: string;
}

export interface Deployment {
  id: number;
  app: string;
  rev: string;
  prev_rev: string;
  status: string;
  started_at: string;
  finished_at: string;
  triggered_by: string;
  env_snapshot: string;
}

export interface RktEvent {
  id: number;
  ts: string;
  kind: string;
  app: string;
  msg: string;
}

export interface User {
  name: string;
  role: string;
  created_at: string;
}

export interface AppMeta {
  name: string;
  runtime?: "managed" | "external";
  external?: boolean;
  command: string;
  args: string[];
  dir: string;
  /** comando sugerido para app externa (solo texto) */
  run_command?: string;
  /** env_file adicional (dotenv) */
  env_file?: string;
  /** dir servido como estático (astro/vite/export); vacío = proceso */
  static?: string;
  restart_delay: string;
  min_uptime: string;
  memory_mb: number;
  port: string;
  domains: string[];
  /** los dominios resuelven a loopback desde esta máquina */
  dns_ok?: boolean;
  deploy: { repo: string; ref: string; build: string[]; auto: boolean } | null;
}

export interface Route {
  hosts: string[];
  target: string;
}

export interface UsageTraffic {
  window_s: number;
  requests: number;
  bytes: number;
  errors: number;
  rps: number;
  err_pct: number;
}

/** Consumo medido de una app (proceso + disco + tráfico real). */
export interface AppUsage {
  app: string;
  pid: number;
  running: boolean;
  http_ok: boolean;
  restarts: number;
  uptime_s: number;
  cpu_pct: number;
  rss_mb: number;
  peak_rss_mb: number;
  virtual_mb: number;
  io_read_mb: number;
  io_write_mb: number;
  handles: number;
  threads: number;
  disk_bytes: number;
  /** el walk de disco se cortó por presupuesto de tiempo */
  disk_partial?: boolean;
  log_bytes: number;
  traffic: UsageTraffic;
}
