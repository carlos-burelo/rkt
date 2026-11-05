// Fixtures compartidos por capturas y guards. Datos neutros, sin marcas.

export const now = Date.now();
export const iso = (minsAgo) => new Date(now - minsAgo * 60000).toISOString();

export const apps = [
  { name: "tienda", running: true, restarts: 0, failed: false },
  { name: "api", running: true, restarts: 0, failed: false },
  { name: "documentacion", running: true, restarts: 1, failed: false },
  { name: "facturacion", running: false, restarts: 7, failed: true },
  { name: "panel", running: true, restarts: 0, failed: false },
  { name: "legado", running: false, restarts: 2, failed: false },
  { name: "vista-previa", running: true, restarts: 0, failed: false },
];

export const metas = [
  { name: "tienda", command: "node .next/standalone/server.js", args: [], dir: "C:/rkt/apps/tienda", restart_delay: "2s", min_uptime: "10s", memory_mb: 512, port: "3001", domains: ["tienda.ejemplo.com"], deploy: { repo: "https://github.com/alex/tienda", ref: "main", build: ["pnpm install", "pnpm build"], auto: true } },
  { name: "api", command: "C:/rkt/apps/api/api.exe", args: [], dir: "C:/rkt/apps/api", restart_delay: "1s", min_uptime: "5s", memory_mb: 256, port: "3002", domains: ["api.ejemplo.com"], deploy: { repo: "https://github.com/alex/api", ref: "main", build: ["go build ./cmd/api"], auto: true } },
  { name: "documentacion", command: "node server.js", args: [], dir: "C:/rkt/apps/documentacion", restart_delay: "2s", min_uptime: "10s", memory_mb: 256, port: "3003", domains: ["documentacion.ejemplo.com", "www.ejemplo.com"], deploy: { repo: "https://github.com/alex/documentacion", ref: "main", build: ["pnpm install", "pnpm build"], auto: false } },
  { name: "facturacion", command: "python worker.py", args: [], dir: "C:/rkt/apps/facturacion", restart_delay: "5s", min_uptime: "30s", memory_mb: 512, port: "3004", domains: [], deploy: { repo: "https://github.com/alex/facturacion", ref: "main", build: ["pip install -r requirements.txt"], auto: true } },
  { name: "panel", command: "node dist/server.js", args: [], dir: "C:/rkt/apps/panel", restart_delay: "2s", min_uptime: "10s", memory_mb: 256, port: "3005", domains: ["panel.ejemplo.com"], deploy: { repo: "https://github.com/alex/panel", ref: "main", build: ["pnpm install", "pnpm build"], auto: false } },
  { name: "legado", command: "C:/rkt/apps/legado/legado.exe", args: [], dir: "C:/rkt/apps/legado", restart_delay: "3s", min_uptime: "15s", memory_mb: 128, port: "3006", domains: [], deploy: null },
  { name: "vista-previa", runtime: "external", external: true, command: "", args: [], dir: "C:/dev/vista-previa", restart_delay: "5s", min_uptime: "10s", memory_mb: 0, port: "3010", domains: ["vista-previa.ejemplo.com"], deploy: null, run_command: "pnpm exec vite --host 127.0.0.1 --port 3010 --strictPort" },
];

export const depSeed = [
  [23, "tienda", "ok", "9f3c2a1", "alex"],
  [22, "api", "ok", "4b8e1d7", "alex"],
  [21, "facturacion", "failed", "c1a9f02", "auto"],
  [20, "documentacion", "ok", "77d2e4a", "alex"],
  [19, "panel", "ok", "2e6b9c4", "auto"],
  [18, "tienda", "ok", "5a1d8f3", "alex"],
  [17, "api", "failed", "e4c7a90", "auto"],
  [16, "documentacion", "ok", "0f8b3d1", "alex"],
  [15, "tienda", "ok", "aa72c65", "auto"],
  [14, "facturacion", "failed", "b3d1e77", "auto"],
  [13, "panel", "ok", "6c4f2b8", "alex"],
  [12, "api", "ok", "d8e2a41", "auto"],
  [11, "documentacion", "ok", "1a9c7f0", "alex"],
  [10, "tienda", "ok", "f2b8d63", "auto"],
  [9, "legado", "failed", "7e1a4c9", "alex"],
  [8, "api", "ok", "3c6f9a2", "auto"],
  [7, "panel", "ok", "8d4b1e6", "alex"],
  [6, "documentacion", "ok", "5f7a2c8", "auto"],
  [5, "tienda", "ok", "c9e3b70", "alex"],
  [4, "facturacion", "ok", "1d8f6a3", "auto"],
  [3, "api", "ok", "6a2c9e5", "auto"],
  [2, "tienda", "ok", "b4d7f12", "alex"],
  [1, "documentacion", "ok", "e8a1c36", "auto"],
  [0.4, "tienda", "running", "pending", "alex"],
];
export const fullRev = (rev) =>
  rev === "pending" ? "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678" : `${rev}${"0123456789abcdef".slice(0, 33)}`;
export const deps = depSeed.map(([mins, app, status, rev, by], i) => {
  const prev = depSeed.slice(0, i).find(([, a, , r]) => a === app && r !== "pending" && r !== "unknown");
  return {
    id: 148 - i,
    app,
    rev: fullRev(rev),
    prev_rev: prev ? fullRev(prev[3]) : "",
    status,
    started_at: iso(mins + 2),
    finished_at: status === "running" ? "" : iso(mins),
    triggered_by: by,
    env_snapshot: "{}",
  };
});

export const evs = [
  { id: 91, ts: iso(3), kind: "deploy", app: "tienda", msg: "deploy ok rev 9f3c2a1" },
  { id: 90, ts: iso(7), kind: "alert", app: "facturacion", msg: "failed · 7 reinicios en 60min" },
  { id: 89, ts: iso(12), kind: "restart", app: "documentacion", msg: "restart manual" },
  { id: 88, ts: iso(18), kind: "deploy", app: "api", msg: "deploy ok rev 4b8e1d7" },
  { id: 87, ts: iso(24), kind: "discover", app: "tienda", msg: "repo detectado · alex/tienda" },
  { id: 86, ts: iso(31), kind: "alert", app: "api", msg: "error_rate 5xx 2.4% en 15min" },
  { id: 85, ts: iso(44), kind: "deploy", app: "documentacion", msg: "deploy ok rev 77d2e4a" },
  { id: 84, ts: iso(58), kind: "died", app: "facturacion", msg: "exit code 1 · reinicio programado" },
  { id: 83, ts: iso(66), kind: "route", app: "documentacion", msg: "TLS emitido para www.ejemplo.com" },
  { id: 82, ts: iso(80), kind: "deploy", app: "panel", msg: "deploy ok rev 2e6b9c4" },
];

export const routes = [
  { hosts: ["tienda.ejemplo.com"], target: "http://127.0.0.1:3001" },
  { hosts: ["api.ejemplo.com"], target: "http://127.0.0.1:3002" },
  { hosts: ["documentacion.ejemplo.com", "www.ejemplo.com"], target: "http://127.0.0.1:3003" },
  { hosts: ["panel.ejemplo.com"], target: "http://127.0.0.1:3005" },
  { hosts: ["legado.ejemplo.com"], target: "http://127.0.0.1:3006" },
  { hosts: ["vista-previa.ejemplo.com"], target: "http://127.0.0.1:3010" },
];

export const proxy = routes.map((r, i) => ({
  ...r,
  ok: i !== 4,
  tls: i === 3 ? ["off"] : [`2026-${String(12 - i).padStart(2, "0")}-14`],
}));

export const dayKeys = Array.from({ length: 7 }, (_, i) => {
  const d = new Date(now - (6 - i) * 86400000);
  return d.toISOString().slice(0, 10);
});
export const byDay = Object.fromEntries(dayKeys.map((k, i) => [k, [812, 934, 1102, 987, 1240, 1388, 642][i]]));

export const analytics = {
  total: 7105,
  bytes: 428_000_000,
  by_class: { "2xx": 6832, "3xx": 141, "4xx": 96, "5xx": 36 },
  by_day: byDay,
  by_path: [
    { k: "/", v: 1840 },
    { k: "/api/tienda", v: 1212 },
    { k: "/pricing", v: 804 },
    { k: "/documentacion/getting-started", v: 611 },
    { k: "/api/health", v: 522 },
    { k: "/blog/edge", v: 318 },
  ],
  by_host: [
    { k: "tienda.ejemplo.com", v: 2914 },
    { k: "api.ejemplo.com", v: 2041 },
    { k: "documentacion.ejemplo.com", v: 1322 },
    { k: "www.ejemplo.com", v: 611 },
    { k: "panel.ejemplo.com", v: 217 },
  ],
  by_ref: [
    { k: "https://news.ycombinator.com", v: 322 },
    { k: "https://github.com", v: 287 },
    { k: "https://www.google.com", v: 198 },
    { k: "direct", v: 1420 },
    { k: "https://x.com", v: 104 },
  ],
  p50: "42ms",
  p95: "188ms",
};

export const usage = {
  days: 30,
  total: 28412,
  bytes: 1_340_000_000,
  err_rate: 0.018,
  p95: "188ms",
  by_day: byDay,
};

export const obsBuckets = Array.from({ length: 6 }, (_, i) => ({
  ts: iso((5 - i) * 60),
  req: [1220, 1488, 1720, 1544, 1890, 1412][i],
  err: [12, 8, 19, 6, 14, 4][i],
}));

export const obs = {
  hours: 6, buckets: obsBuckets,
  classes: { "2xx": 9120, "3xx": 180, "4xx": 156, "5xx": 63 },
  by_app: [
    { app: "tienda", req: 4210, err: 18, bytes: 18400000, p95: "28ms" },
    { app: "api", req: 3120, err: 41, bytes: 9200000, p95: "64ms" },
    { app: "documentacion", req: 1630, err: 2, bytes: 4100000, p95: "19ms" },
    { app: "panel", req: 559, err: 2, bytes: 1200000, p95: "31ms" },
  ],
  p50: "12ms", p95: "28ms", p99: "91ms",
  events: { deploy: 4, died: 2, alert: 1 },
  restarts: 3, down: ["facturacion"],
};

export const serverHistory = Array.from({ length: 12 }, (_, i) => ({
  ts: iso((11 - i) * 5),
  cpu: [18, 24, 31, 27, 44, 52, 38, 41, 33, 29, 36, 42][i],
  mem: [54, 54, 55, 55, 56, 56, 57, 57, 58, 58, 59, 59][i],
  disk: 38,
  rx_rate: 180000 + i * 9000,
  tx_rate: 90000 + i * 4000,
  load1: [0.4, 0.5, 0.6, 0.5, 0.8, 1.0, 0.7, 0.8, 0.6, 0.5, 0.7, 0.8][i],
  temp_max: [42, 43, 45, 44, 48, 51, 47, 48, 46, 45, 47, 49][i],
}));

export const serverSnapshot = {
  ts: new Date().toISOString(),
  hostname: "rkt-vps-01",
  os: "linux",
  uptime_s: 17 * 86400 + 5 * 3600 + 23 * 60,
  cpu: { percent: 42, cores: 4, per_core: [38, 45, 40, 47] },
  load: { l1: 0.8, l5: 0.65, l15: 0.5, supported: true },
  memory: {
    total: 8 * 1024 ** 3, used: 4.7 * 1024 ** 3, available: 3.3 * 1024 ** 3,
    cached: 1.2 * 1024 ** 3, swap_total: 2 * 1024 ** 3, swap_used: 0.3 * 1024 ** 3,
    percent: 59,
  },
  disks: [
    { mount: "/", fs_type: "ext4", total: 80 * 1024 ** 3, used: 30.4 * 1024 ** 3, free: 49.6 * 1024 ** 3, percent: 38 },
    { mount: "/var/lib/rkt", fs_type: "ext4", total: 120 * 1024 ** 3, used: 54 * 1024 ** 3, free: 66 * 1024 ** 3, percent: 45 },
  ],
  network: { rx_bytes: 184e9, tx_bytes: 91e9, rx_rate: 278000, tx_rate: 134000 },
  temps: [
    { zone: "cpu0", celsius: 49 },
    { zone: "nvme0", celsius: 38 },
  ],
  firewall: { available: true, enabled: true, profiles: ["ufw activo", "22/tcp permitido", "80,443/tcp permitido"] },
  warnings: [],
};

export const serverProcs = [
  { pid: 1842, name: "node", rss_mb: 412.3, cpu_pct: 18.4, threads: 24 },
  { pid: 2210, name: "go", rss_mb: 128.7, cpu_pct: 9.1, threads: 12 },
  { pid: 901, name: "postgres", rss_mb: 246.5, cpu_pct: 4.2, threads: 8 },
  { pid: 640, name: "caddy", rss_mb: 42.8, cpu_pct: 1.8, threads: 10 },
];

export const settings = {
  node_bin: "C:/Program Files/nodejs/node.exe",
  python_bin: "C:/Python312/python.exe",
  base_domain: "ejemplo.com",
  port_pool: "3000-3999",
  site_name: "rkt",
  quota_reqs: "50000",
  quota_bytes: "2147483648",
  public_ip: "203.0.113.7",
};

export const users = [
  { name: "alex", role: "admin", created_at: iso(60 * 24 * 90), last_login: iso(2), disabled: false },
  { name: "dani", role: "operator", created_at: iso(60 * 24 * 31), last_login: iso(60 * 24), disabled: false },
  { name: "noa", role: "viewer", created_at: iso(60 * 24 * 12), last_login: iso(60 * 24 * 8), disabled: true },
];

export const channels = [
  { id: 1, name: "ops-webhook", url: "https://hooks.example.com/rkt", enabled: true, created_at: iso(60 * 24 * 10) },
];

export const fires = [
  { id: 9, rule_id: 4, ts: iso(7), value: 3, threshold: 1, message: "regla #4 · failed 3 ≥ 1", resolved_at: "" },
  { id: 8, rule_id: 4, ts: iso(60 * 3), value: 1, threshold: 1, message: "regla #4 · failed 1 ≥ 1", resolved_at: "" },
];

export const audit = [
  { id: 4, ts: iso(2), actor: "alex", action: "login", target: "alex", ip: "127.0.0.1" },
  { id: 3, ts: iso(31), actor: "alex", action: "user.role", target: "dani", ip: "127.0.0.1" },
  { id: 2, ts: iso(60 * 8), actor: "alex", action: "session.revoke", target: "noa", ip: "127.0.0.1" },
  { id: 1, ts: iso(60 * 24 * 3), actor: "alex", action: "user.create", target: "noa", ip: "127.0.0.1" },
];

export const alerts = [
  { id: 5, app: "*", kind: "cpu_pct", threshold: 85, window_min: 15, enabled: true, channel_id: 1, last_fired: "", created_at: iso(60 * 24 * 2) },
  { id: 4, app: "facturacion", kind: "failed", threshold: 1, window_min: 60, enabled: true, channel_id: 1, last_fired: iso(7), created_at: iso(60 * 24 * 20) },
  { id: 3, app: "api", kind: "error_rate", threshold: 0.02, window_min: 15, enabled: true, channel_id: 1, last_fired: iso(31), created_at: iso(60 * 24 * 16) },
  { id: 2, app: "*", kind: "latency_p95", threshold: 250, window_min: 60, enabled: true, channel_id: 0, last_fired: "", created_at: iso(60 * 24 * 9) },
  { id: 1, app: "*", kind: "failed", threshold: 2, window_min: 30, enabled: false, channel_id: 0, last_fired: "", created_at: iso(60 * 24 * 4) },
];

export const envs = {
  tienda: { NODE_ENV: "production", STRIPE_KEY: "sk_live_51N...", PUBLIC_URL: "https://tienda.ejemplo.com", REDIS_URL: "redis://127.0.0.1:6379" },
  "api": { DATABASE_URL: "postgres://rkt:...@127.0.0.1:5432/rkt", JWT_SECRET: "9f3c...", LOG_LEVEL: "info" },
  documentacion: { PUBLIC_URL: "https://documentacion.ejemplo.com", NODE_ENV: "production" },
  "facturacion": { DATABASE_URL: "postgres://rkt:...@127.0.0.1:5432/rkt", STRIPE_KEY: "sk_live_51N...", SENTRY_DSN: "https://..." },
  panel: { NODE_ENV: "production", SESSION_SECRET: "b4d7..." },
  legado: {},
};

export const buildLog = [
  "▶ step: install",
  "✔ step ok: install",
  "▶ step: build",
  "pnpm build",
  "vite v6.4.5 building for production...",
  "✓ 2647 modules transformed.",
  "dist/assets/index-CptZgEO5.js   764.75 kB │ gzip: 235.77 kB",
  "✔ step ok: build",
  "▶ step: package",
  "✔ step ok: package",
  "deploy ok · rev 9f3c2a1 · 38s",
].join("\n");

export const logText = Array.from({ length: 120 }, (_, i) => {
  const level = i % 37 === 0 ? "ERROR" : i % 17 === 0 ? "WARN" : "INFO";
  const msg = level === "ERROR"
    ? "tienda: upstream timeout after 2000ms path=/api/tienda"
    : level === "WARN"
      ? "tienda: slow query 412ms path=/api/tienda"
      : `tienda: request ok path=/api/tienda status=200 dur=${18 + (i % 44)}ms`;
  return `time=2026-11-02T${String(9 + Math.floor(i / 12)).padStart(2, "0")}:${String((i * 5) % 60).padStart(2, "0")}:00Z level=${level} ${msg}`;
}).join("\n");

