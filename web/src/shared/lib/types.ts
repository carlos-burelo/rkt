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
}

export interface Deployment {
  id: number;
  app: string;
  rev: string;
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
  command: string;
  args: string[];
  dir: string;
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
