// Capturas con API mockeada (sin rkt corriendo). Uso local:
// pnpm --dir web dev --port 5173
// node scripts/shots/mock.mjs
import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const OUT = join(ROOT, "docs", "shots");
const BASE = process.env.RKT_URL ?? "http://localhost:5173";
const now = Date.now();
const iso = (minsAgo) => new Date(now - minsAgo * 60000).toISOString();

const apps = [
  { name: "checkout", running: true, restarts: 0, failed: false },
  { name: "api-core", running: true, restarts: 0, failed: false },
  { name: "docs", running: true, restarts: 1, failed: false },
  { name: "billing-worker", running: false, restarts: 7, failed: true },
  { name: "admin", running: true, restarts: 0, failed: false },
  { name: "legacy", running: false, restarts: 2, failed: false },
  { name: "edge-preview", running: true, restarts: 0, failed: false },
];

const metas = [
  { name: "checkout", command: "node .next/standalone/server.js", args: [], dir: "C:/rkt/apps/checkout", restart_delay: "2s", min_uptime: "10s", memory_mb: 512, port: "3001", domains: ["checkout.rkt.dev"], deploy: { repo: "https://github.com/xavi/checkout", ref: "main", build: ["pnpm install", "pnpm build"], auto: true } },
  { name: "api-core", command: "C:/rkt/apps/api-core/api-core.exe", args: [], dir: "C:/rkt/apps/api-core", restart_delay: "1s", min_uptime: "5s", memory_mb: 256, port: "3002", domains: ["api.rkt.dev"], deploy: { repo: "https://github.com/xavi/api-core", ref: "main", build: ["go build ./cmd/api"], auto: true } },
  { name: "docs", command: "node server.js", args: [], dir: "C:/rkt/apps/docs", restart_delay: "2s", min_uptime: "10s", memory_mb: 256, port: "3003", domains: ["docs.rkt.dev", "www.rkt.dev"], deploy: { repo: "https://github.com/xavi/docs", ref: "main", build: ["pnpm install", "pnpm build"], auto: false } },
  { name: "billing-worker", command: "python worker.py", args: [], dir: "C:/rkt/apps/billing", restart_delay: "5s", min_uptime: "30s", memory_mb: 512, port: "3004", domains: [], deploy: { repo: "https://github.com/xavi/billing", ref: "main", build: ["pip install -r requirements.txt"], auto: true } },
  { name: "admin", command: "node dist/server.js", args: [], dir: "C:/rkt/apps/admin", restart_delay: "2s", min_uptime: "10s", memory_mb: 256, port: "3005", domains: ["admin.rkt.dev"], deploy: { repo: "https://github.com/xavi/admin", ref: "main", build: ["pnpm install", "pnpm build"], auto: false } },
  { name: "legacy", command: "C:/rkt/apps/legacy/legacy.exe", args: [], dir: "C:/rkt/apps/legacy", restart_delay: "3s", min_uptime: "15s", memory_mb: 128, port: "3006", domains: [], deploy: null },
  { name: "edge-preview", runtime: "external", external: true, command: "", args: [], dir: "C:/dev/edge-preview", restart_delay: "5s", min_uptime: "10s", memory_mb: 0, port: "3010", domains: ["preview.rkt.dev"], deploy: null, run_command: "pnpm exec vite --host 127.0.0.1 --port 3010 --strictPort" },
];

const depSeed = [
  [23, "checkout", "ok", "9f3c2a1", "xavi"],
  [22, "api-core", "ok", "4b8e1d7", "xavi"],
  [21, "billing-worker", "failed", "c1a9f02", "auto"],
  [20, "docs", "ok", "77d2e4a", "xavi"],
  [19, "admin", "ok", "2e6b9c4", "auto"],
  [18, "checkout", "ok", "5a1d8f3", "xavi"],
  [17, "api-core", "failed", "e4c7a90", "auto"],
  [16, "docs", "ok", "0f8b3d1", "xavi"],
  [15, "checkout", "ok", "aa72c65", "auto"],
  [14, "billing-worker", "failed", "b3d1e77", "auto"],
  [13, "admin", "ok", "6c4f2b8", "xavi"],
  [12, "api-core", "ok", "d8e2a41", "auto"],
  [11, "docs", "ok", "1a9c7f0", "xavi"],
  [10, "checkout", "ok", "f2b8d63", "auto"],
  [9, "legacy", "failed", "7e1a4c9", "xavi"],
  [8, "api-core", "ok", "3c6f9a2", "auto"],
  [7, "admin", "ok", "8d4b1e6", "xavi"],
  [6, "docs", "ok", "5f7a2c8", "auto"],
  [5, "checkout", "ok", "c9e3b70", "xavi"],
  [4, "billing-worker", "ok", "1d8f6a3", "auto"],
  [3, "api-core", "ok", "6a2c9e5", "auto"],
  [2, "checkout", "ok", "b4d7f12", "xavi"],
  [1, "docs", "ok", "e8a1c36", "auto"],
  [0.4, "checkout", "running", "pending", "xavi"],
];
const fullRev = (rev) =>
  rev === "pending" ? "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678" : `${rev}${"0123456789abcdef".slice(0, 33)}`;
const deps = depSeed.map(([mins, app, status, rev, by], i) => {
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

const evs = [
  { id: 91, ts: iso(3), kind: "deploy", app: "checkout", msg: "deploy ok rev 9f3c2a1" },
  { id: 90, ts: iso(7), kind: "alert", app: "billing-worker", msg: "failed · 7 reinicios en 60min" },
  { id: 89, ts: iso(12), kind: "restart", app: "docs", msg: "restart manual" },
  { id: 88, ts: iso(18), kind: "deploy", app: "api-core", msg: "deploy ok rev 4b8e1d7" },
  { id: 87, ts: iso(24), kind: "discover", app: "checkout", msg: "repo detectado · xavi/checkout" },
  { id: 86, ts: iso(31), kind: "alert", app: "api-core", msg: "error_rate 5xx 2.4% en 15min" },
  { id: 85, ts: iso(44), kind: "deploy", app: "docs", msg: "deploy ok rev 77d2e4a" },
  { id: 84, ts: iso(58), kind: "died", app: "billing-worker", msg: "exit code 1 · reinicio programado" },
  { id: 83, ts: iso(66), kind: "route", app: "docs", msg: "TLS emitido para www.rkt.dev" },
  { id: 82, ts: iso(80), kind: "deploy", app: "admin", msg: "deploy ok rev 2e6b9c4" },
];

const routes = [
  { hosts: ["checkout.rkt.dev"], target: "http://127.0.0.1:3001" },
  { hosts: ["api.rkt.dev"], target: "http://127.0.0.1:3002" },
  { hosts: ["docs.rkt.dev", "www.rkt.dev"], target: "http://127.0.0.1:3003" },
  { hosts: ["admin.rkt.dev"], target: "http://127.0.0.1:3005" },
  { hosts: ["legacy.rkt.dev"], target: "http://127.0.0.1:3006" },
  { hosts: ["preview.rkt.dev"], target: "http://127.0.0.1:3010" },
];

const proxy = routes.map((r, i) => ({
  ...r,
  ok: i !== 4,
  tls: i === 3 ? ["off"] : [`2026-${String(12 - i).padStart(2, "0")}-14`],
}));

const dayKeys = Array.from({ length: 7 }, (_, i) => {
  const d = new Date(now - (6 - i) * 86400000);
  return d.toISOString().slice(0, 10);
});
const byDay = Object.fromEntries(dayKeys.map((k, i) => [k, [812, 934, 1102, 987, 1240, 1388, 642][i]]));

const analytics = {
  total: 7105,
  bytes: 428_000_000,
  by_class: { "2xx": 6832, "3xx": 141, "4xx": 96, "5xx": 36 },
  by_day: byDay,
  by_path: [
    { k: "/", v: 1840 },
    { k: "/api/checkout", v: 1212 },
    { k: "/pricing", v: 804 },
    { k: "/docs/getting-started", v: 611 },
    { k: "/api/health", v: 522 },
    { k: "/blog/edge", v: 318 },
  ],
  by_host: [
    { k: "checkout.rkt.dev", v: 2914 },
    { k: "api.rkt.dev", v: 2041 },
    { k: "docs.rkt.dev", v: 1322 },
    { k: "www.rkt.dev", v: 611 },
    { k: "admin.rkt.dev", v: 217 },
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

const usage = {
  days: 30,
  total: 28412,
  bytes: 1_340_000_000,
  err_rate: 0.018,
  p95: "188ms",
  by_day: byDay,
};

const obsBuckets = Array.from({ length: 6 }, (_, i) => ({
  ts: iso((5 - i) * 60),
  req: [1220, 1488, 1720, 1544, 1890, 1412][i],
  err: [12, 8, 19, 6, 14, 4][i],
}));

const obs = {
  hours: 6, buckets: obsBuckets,
  classes: { "2xx": 9120, "3xx": 180, "4xx": 156, "5xx": 63 },
  by_app: [
    { app: "checkout", req: 4210, err: 18, bytes: 18400000, p95: "28ms" },
    { app: "api-core", req: 3120, err: 41, bytes: 9200000, p95: "64ms" },
    { app: "docs", req: 1630, err: 2, bytes: 4100000, p95: "19ms" },
    { app: "admin", req: 559, err: 2, bytes: 1200000, p95: "31ms" },
  ],
  p50: "12ms", p95: "28ms", p99: "91ms",
  restarts: 3, down: ["billing-worker"],
};

const settings = {
  node_bin: "C:/Program Files/nodejs/node.exe",
  python_bin: "C:/Python312/python.exe",
  base_domain: "rkt.dev",
  port_pool: "3000-3999",
  site_name: "rkt",
  quota_reqs: "50000",
  quota_bytes: "2147483648",
  public_ip: "203.0.113.7",
};

const users = [
  { name: "xavi", role: "admin", created_at: iso(60 * 24 * 90), last_login: iso(2), disabled: false },
  { name: "mira", role: "operator", created_at: iso(60 * 24 * 31), last_login: iso(60 * 24), disabled: false },
  { name: "tomas", role: "viewer", created_at: iso(60 * 24 * 12), last_login: iso(60 * 24 * 8), disabled: true },
];

const channels = [
  { id: 1, name: "ops-webhook", url: "https://hooks.example.com/rkt", enabled: true, created_at: iso(60 * 24 * 10) },
];

const fires = [
  { id: 9, rule_id: 4, ts: iso(7), value: 3, threshold: 1, message: "regla #4 · failed 3 ≥ 1", resolved_at: "" },
  { id: 8, rule_id: 4, ts: iso(60 * 3), value: 1, threshold: 1, message: "regla #4 · failed 1 ≥ 1", resolved_at: "" },
];

const audit = [
  { id: 4, ts: iso(2), actor: "xavi", action: "login", target: "xavi", ip: "127.0.0.1" },
  { id: 3, ts: iso(31), actor: "xavi", action: "user.role", target: "mira", ip: "127.0.0.1" },
  { id: 2, ts: iso(60 * 8), actor: "xavi", action: "session.revoke", target: "tomas", ip: "127.0.0.1" },
  { id: 1, ts: iso(60 * 24 * 3), actor: "xavi", action: "user.create", target: "tomas", ip: "127.0.0.1" },
];

const alerts = [
  { id: 4, app: "billing-worker", kind: "failed", threshold: 1, window_min: 60, enabled: true, channel_id: 1, last_fired: iso(7), created_at: iso(60 * 24 * 20) },
  { id: 3, app: "api-core", kind: "error_rate", threshold: 0.02, window_min: 15, enabled: true, channel_id: 1, last_fired: iso(31), created_at: iso(60 * 24 * 16) },
  { id: 2, app: "*", kind: "latency_p95", threshold: 250, window_min: 60, enabled: true, channel_id: 0, last_fired: "", created_at: iso(60 * 24 * 9) },
  { id: 1, app: "*", kind: "failed", threshold: 2, window_min: 30, enabled: false, channel_id: 0, last_fired: "", created_at: iso(60 * 24 * 4) },
];

const envs = {
  checkout: { NODE_ENV: "production", STRIPE_KEY: "sk_live_51N...", PUBLIC_URL: "https://checkout.rkt.dev", REDIS_URL: "redis://127.0.0.1:6379" },
  "api-core": { DATABASE_URL: "postgres://rkt:...@127.0.0.1:5432/rkt", JWT_SECRET: "9f3c...", LOG_LEVEL: "info" },
  docs: { PUBLIC_URL: "https://docs.rkt.dev", NODE_ENV: "production" },
  "billing-worker": { DATABASE_URL: "postgres://rkt:...@127.0.0.1:5432/rkt", STRIPE_KEY: "sk_live_51N...", SENTRY_DSN: "https://..." },
  admin: { NODE_ENV: "production", SESSION_SECRET: "b4d7..." },
  legacy: {},
};

const buildLog = [
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

const logText = Array.from({ length: 120 }, (_, i) => {
  const level = i % 37 === 0 ? "ERROR" : i % 17 === 0 ? "WARN" : "INFO";
  const msg = level === "ERROR"
    ? "checkout: upstream timeout after 2000ms path=/api/checkout"
    : level === "WARN"
      ? "checkout: slow query 412ms path=/api/checkout"
      : `checkout: request ok path=/api/checkout status=200 dur=${18 + (i % 44)}ms`;
  return `time=2026-11-02T${String(9 + Math.floor(i / 12)).padStart(2, "0")}:${String((i * 5) % 60).padStart(2, "0")}:00Z level=${level} ${msg}`;
}).join("\n");

function json(route, body, status = 200) {
  return route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
}

async function mockApi(page, { authed = true, setupRequired = false } = {}) {
  await page.route("**/api/v1/**", (route) => {
    const url = new URL(route.request().url());
    const p = url.pathname;
    const method = route.request().method();

    if (p === "/api/v1/auth/status") return json(route, { setup_required: setupRequired });
    if (p === "/api/v1/auth/me") {
      return authed ? json(route, { user: "xavi", role: "admin" }) : json(route, { error: "no autorizado" }, 401);
    }
    if (p === "/api/v1/auth/login" || p === "/api/v1/auth/setup") return json(route, { user: "xavi", role: "admin" });
    if (p === "/api/v1/auth/logout") return json(route, { ok: "bye" });
    if (p === "/api/v1/status") return json(route, { version: "0.6.6", routes: routes.length, apps });
    if (p === "/api/v1/deployments") return json(route, deps);
    if (p.startsWith("/api/v1/deployments/") && p.endsWith("/log")) return json(route, { log: buildLog });
    if (p.startsWith("/api/v1/deployments/") && p.endsWith("/retry")) return json(route, { rev: "9f3c2a1" });
    if (p.startsWith("/api/v1/deployments/") && p.endsWith("/cancel")) return json(route, { ok: "true" });
    if (p === "/api/v1/events") return json(route, evs);
    if (p === "/api/v1/apps") return json(route, metas);
    if (p === "/api/v1/apps" && method === "POST") return json(route, { app: "nueva-app" });
    if (p.startsWith("/api/v1/apps/") && p.endsWith("/diff")) {
      return json(route, {
        from: "5a1d8f3",
        to: "9f3c2a1",
        files: [
          { status: "M", path: "app/page.tsx" },
          { status: "A", path: "public/og.png" },
          { status: "D", path: "src/legacy.ts" },
          { status: "R100", from: "src/client.ts", path: "src/api/client.ts" },
        ],
      });
    }
    if (p.startsWith("/api/v1/apps/") && p.endsWith("/env")) {
      const name = decodeURIComponent(p.split("/")[4] ?? "");
      if (method === "GET") return json(route, Object.keys(envs[name] ?? {}));
      return json(route, { ok: true, restart: true });
    }
    if (p.startsWith("/api/v1/apps/") && p.endsWith("/env/import")) return json(route, { ok: true, keys: ["IMPORTED"] });
    if (p.startsWith("/api/v1/apps/") && p.includes("/env/")) {
      const parts = p.split("/");
      const name = decodeURIComponent(parts[4] ?? "");
      const key = decodeURIComponent(parts[6] ?? "");
      return json(route, { key, value: envs[name]?.[key] ?? "" });
    }
    if (p.startsWith("/api/v1/apps/") && /\/apps\/[^/]+\/(start|stop)$/.test(p)) return json(route, { ok: true });
    if (p.startsWith("/api/v1/apps/") && method !== "GET") return json(route, {});
    if (p === "/api/v1/proxy") return json(route, proxy);
    if (p === "/api/v1/routes") return json(route, method === "GET" ? routes : {});
    if (p === "/api/v1/domains/zones") return json(route, { suffixes: ["localhost", "test"], groups: [] });
    if (p === "/api/v1/domains/suggest") return json(route, { suggestions: [] });
    if (p === "/api/v1/env") {
      const apps = {};
      for (const [name, vars] of Object.entries(envs)) apps[name] = Object.keys(vars);
      return json(route, { global: ["LOG_LEVEL", "SENTRY_DSN"], apps });
    }
    if (p === "/api/v1/env/global") return json(route, { ok: true, restart: true });
    if (p === "/api/v1/env/global/import") return json(route, { ok: true, keys: ["IMPORTED"] });
    if (p.startsWith("/api/v1/env/global/")) {
      const key = decodeURIComponent(p.split("/").pop() ?? "");
      return json(route, { key, value: { LOG_LEVEL: "info", SENTRY_DSN: "https://..." }[key] ?? "" });
    }
    if (p.startsWith("/api/v1/domains/")) {
      const host = decodeURIComponent(p.slice("/api/v1/domains/".length));
      const routeDef = routes.find((r) => r.hosts.includes(host));
      return json(route, {
        host,
        target: routeDef?.target ?? "http://127.0.0.1:3001",
        app: metas.find((m) => (m.domains ?? []).includes(host))?.name ?? "",
        tls: { managed: true, expires: host.includes("docs") ? "2026-10-14" : "2026-12-14" },
        dns: { a: ["127.0.0.1"], aaaa: [], resolves: true },
        expected: "127.0.0.1",
        dns_ok: true,
        checked_at: new Date().toISOString(),
      });
    }
    if (p === "/api/v1/analytics") return json(route, analytics);
    if (p === "/api/v1/usage") return json(route, usage);
    if (p === "/api/v1/observability") return json(route, obs);
    if (p === "/api/v1/settings") return json(route, settings);
    if (p === "/api/v1/github") return json(route, { configured: false });
    if (p === "/api/v1/github/repos") return json(route, []);
    if (p === "/api/v1/users") return json(route, users);
    if (p === "/api/v1/audit") return json(route, audit);
    if (p.startsWith("/api/v1/users/") && p.endsWith("/sessions")) {
      return json(route, {
        current: "sess-current",
        sessions: [
          { id: "sess-current", user: "xavi", created_at: iso(2), expires_at: new Date(Date.now() + 30 * 86400000).toISOString() },
          { id: "sess-old", user: "xavi", created_at: iso(60 * 24 * 9), expires_at: new Date(Date.now() + 12 * 86400000).toISOString() },
        ],
      });
    }
    if (p === "/api/v1/alerts") return json(route, method === "GET" ? alerts : { id: 5 });
    if (p === "/api/v1/alert-channels") return json(route, method === "GET" ? channels : { id: 2 });
    if (p.startsWith("/api/v1/alert-channels/")) return json(route, { ok: true });
    if (p.endsWith("/fires")) return json(route, fires);
    if (p.endsWith("/test")) return json(route, { ok: true, message: "regla de prueba" });
    if (p.startsWith("/api/v1/alerts/")) return json(route, { ok: true });
    if (p === "/api/v1/logs/stream") {
      const body = `event: init\ndata: ${JSON.stringify({ log: logText })}\n\nevent: end\ndata: {"status":"done"}\n\n`;
      return route.fulfill({ status: 200, contentType: "text/event-stream", body });
    }
    if (p === "/api/v1/logs") return json(route, { log: logText });
    return json(route, { error: `mock sin ruta: ${p}` }, 404);
  });
}

const main = async () => {
  mkdirSync(OUT, { recursive: true });
  const browser = await chromium.launch();
  const errors = [];

  const ctx = await browser.newContext({ viewport: { width: 1600, height: 900 }, deviceScaleFactor: 1, colorScheme: "dark" });
  const page = await ctx.newPage();
  await mockApi(page);
  page.on("console", (m) => { if (m.type() === "error") errors.push(`[consola] ${m.text()}`); });
  page.on("pageerror", (e) => errors.push(`[pageerror] ${e.message}`));
  page.on("response", (r) => { if (r.status() >= 400) errors.push(`[http ${r.status()}] ${r.url()}`); });

  const shot = async (name, route, opts = {}) => {
    await page.goto(`${BASE}/${route}`, { waitUntil: "networkidle" });
    await page.waitForTimeout(500);
    await page.screenshot({ path: join(OUT, `${name}.png`), fullPage: opts.fullPage ?? true });
    console.log("ok", name);
  };

  await shot("overview", "overview");
  await shot("deployments", "deployments");
  await shot("ci", "ci");
  await shot("logs", "logs");
  await shot("routes", "routes");
  await shot("proxy", "proxy");
  await shot("analytics", "analytics");
  await shot("observability", "observability");
  await shot("alerts", "alerts");
  await shot("environment", "environment");
  await shot("settings", "settings");
  await shot("users", "users");
  for (const tab of ["overview", "deploys", "logs", "analytics", "env", "settings"]) {
    await shot(`app-${tab}`, `app/checkout/${tab}`);
  }
  await shot("app-external", "app/edge-preview/overview");

  // overlays desktop
  await page.goto(`${BASE}/overview`, { waitUntil: "networkidle" });
  await page.waitForTimeout(300);
  await page.keyboard.press("Control+k");
  await page.waitForTimeout(250);
  await page.screenshot({ path: join(OUT, "overlay-palette.png"), fullPage: false });
  console.log("ok overlay-palette");
  await page.keyboard.press("Escape");

  await page.goto(`${BASE}/deployments`, { waitUntil: "networkidle" });
  await page.waitForTimeout(300);
  await page.locator('[role="row"]').filter({ hasText: "#143" }).click();
  await page.waitForTimeout(400);
  await page.screenshot({ path: join(OUT, "overlay-drawer.png"), fullPage: false });
  console.log("ok overlay-drawer");

  await page.goto(`${BASE}/overview`, { waitUntil: "networkidle" });
  await page.waitForTimeout(300);
  await page.getByRole("button", { name: /Nueva app/ }).first().click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: join(OUT, "overlay-create.png"), fullPage: false });
  console.log("ok overlay-create");
  await ctx.close();

  // mobile
  const mctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, colorScheme: "dark" });
  const mpage = await mctx.newPage();
  await mockApi(mpage);
  mpage.on("console", (m) => { if (m.type() === "error") errors.push(`[móvil consola] ${m.text()}`); });
  mpage.on("pageerror", (e) => errors.push(`[móvil pageerror] ${e.message}`));
  mpage.on("response", (r) => { if (r.status() >= 400) errors.push(`[móvil http ${r.status()}] ${r.url()}`); });
  for (const [name, route] of [
    ["mobile-overview", "overview"],
    ["mobile-deployments", "deployments"],
    ["mobile-app-overview", "app/checkout/overview"],
    ["mobile-proxy", "proxy"],
    ["mobile-logs", "logs"],
  ]) {
    await mpage.goto(`${BASE}/${route}`, { waitUntil: "networkidle" });
    await mpage.waitForTimeout(500);
    await mpage.screenshot({ path: join(OUT, `${name}.png`), fullPage: true });
    console.log("ok", name);
  }
  await mctx.close();

  // login + setup sin sesión
  const lctx = await browser.newContext({ viewport: { width: 1600, height: 900 }, colorScheme: "dark" });
  const lpage = await lctx.newPage();
  await mockApi(lpage, { authed: false });
  await lpage.goto(`${BASE}/login`, { waitUntil: "networkidle" });
  await lpage.waitForTimeout(400);
  await lpage.screenshot({ path: join(OUT, "login.png"), fullPage: true });
  console.log("ok login");
  await lctx.close();

  const sctx = await browser.newContext({ viewport: { width: 1600, height: 900 }, colorScheme: "dark" });
  const spage = await sctx.newPage();
  await mockApi(spage, { authed: false, setupRequired: true });
  await spage.goto(`${BASE}/setup`, { waitUntil: "networkidle" });
  await spage.waitForTimeout(400);
  await spage.screenshot({ path: join(OUT, "setup.png"), fullPage: true });
  console.log("ok setup");
  await sctx.close();

  // tema claro
  const wctx = await browser.newContext({ viewport: { width: 1600, height: 900 }, colorScheme: "light" });
  const wpage = await wctx.newPage();
  await mockApi(wpage);
  wpage.on("console", (m) => { if (m.type() === "error") errors.push(`[light consola] ${m.text()}`); });
  wpage.on("pageerror", (e) => errors.push(`[light pageerror] ${e.message}`));
  wpage.on("response", (r) => { if (r.status() >= 400) errors.push(`[light http ${r.status()}] ${r.url()}`); });
  await wpage.goto(`${BASE}/overview`, { waitUntil: "networkidle" });
  await wpage.waitForTimeout(500);
  await wpage.screenshot({ path: join(OUT, "overview-light.png"), fullPage: true });
  console.log("ok overview-light");
  await wctx.close();

  const wlctx = await browser.newContext({ viewport: { width: 1600, height: 900 }, colorScheme: "light" });
  const wlpage = await wlctx.newPage();
  await mockApi(wlpage, { authed: false });
  await wlpage.goto(`${BASE}/login`, { waitUntil: "networkidle" });
  await wlpage.waitForTimeout(400);
  await wlpage.screenshot({ path: join(OUT, "login-light.png"), fullPage: true });
  console.log("ok login-light");
  await wlctx.close();

  await browser.close();
  if (errors.length) {
    console.log("\nErrores de consola:");
    for (const e of [...new Set(errors)]) console.log(e);
    process.exitCode = 1;
  } else {
    console.log("\nsin errores de consola");
  }
  console.log("listo en docs/shots/");
};

main().catch((e) => {
  console.error("mock shots:", e);
  process.exit(1);
});
