// Capturas con API mockeada (sin rkt corriendo). Uso local:
// pnpm --dir web dev --port 5173
// node scripts/shots/mock.mjs
import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import { now, iso, apps, metas, depSeed, fullRev, deps, evs, routes, proxy, dayKeys, byDay, analytics, usage, obsBuckets, obs, serverHistory, serverSnapshot, serverProcs, cronJobs, cronRuns, cronLog, settings, users, channels, fires, audit, alerts, envs, buildLog, logText } from "./fixtures.mjs";

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const OUT = join(ROOT, "docs", "shots");
const BASE = process.env.RKT_URL ?? "http://localhost:5173";
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
      return authed ? json(route, { user: "alex", role: "admin" }) : json(route, { error: "no autorizado" }, 401);
    }
    if (p === "/api/v1/auth/login" || p === "/api/v1/auth/setup") return json(route, { user: "alex", role: "admin" });
    if (p === "/api/v1/auth/logout") return json(route, { ok: "bye" });
    if (p === "/api/v1/status") return json(route, { version: "0.6.6", routes: routes.length, apps });
    if (p === "/api/v1/deployments") return json(route, deps);
    if (p.startsWith("/api/v1/deployments/") && p.endsWith("/log")) {
      if (url.searchParams.get("stream") === "1") {
        const init = { log: "▶ step: pnpm install\n✔ step ok: pnpm install (1.2s)\n▶ step: pnpm run build\n" };
        const chunk = { chunk: "✔ step ok: pnpm run build (8.4s)\n▶ step: health\n" };
        const body = `event: init\ndata: ${JSON.stringify(init)}\n\nevent: chunk\ndata: ${JSON.stringify(chunk)}\n\n`;
        return route.fulfill({ status: 200, contentType: "text/event-stream", body });
      }
      return json(route, { log: buildLog });
    }
    if (p.startsWith("/api/v1/deployments/") && p.endsWith("/retry")) return json(route, { rev: "9f3c2a1" });
    if (p.startsWith("/api/v1/deployments/") && p.endsWith("/cancel")) return json(route, { ok: "true" });
    if (/^\/api\/v1\/deployments\/\d+$/.test(p) && method === "GET") {
      const id = Number(p.split("/").pop());
      const found = deps.find((d) => d.id === id);
      if (found) return json(route, found);
      return json(route, {
        id, app: "nueva-app", rev: fullRev("pending"), prev_rev: "",
        status: "running", started_at: new Date().toISOString(), finished_at: "",
        triggered_by: "alex", env_snapshot: "{}",
      });
    }
    if (p === "/api/v1/events") return json(route, evs);
    if (p === "/api/v1/apps" && method === "GET") return json(route, metas);
    if (p === "/api/v1/apps" && method === "POST") return json(route, { app: "nueva-app" });
    if (p.startsWith("/api/v1/apps/") && p.endsWith("/diff")) {
      return json(route, {
        from: "5a1d8f3",
        to: "9f3c2a1",
        files: [
          { status: "M", path: "app/page.tsx" },
          { status: "A", path: "public/og.png" },
          { status: "D", path: "src/legado.ts" },
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
    if (/^\/api\/v1\/apps\/[^/]+\/build$/.test(p) && method === "POST") return json(route, { deployment: 900 });
    if (/^\/api\/v1\/apps\/[^/]+\/deploy$/.test(p) && method === "POST") return json(route, { deployment: 901 });
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
        tls: { managed: true, expires: host.includes("documentacion") ? "2026-10-14" : "2026-12-14" },
        dns: { a: ["127.0.0.1"], aaaa: [], resolves: true },
        expected: "127.0.0.1",
        dns_ok: true,
        checked_at: new Date().toISOString(),
      });
    }
    if (p === "/api/v1/analytics") return json(route, analytics);
    if (p === "/api/v1/usage") return json(route, usage);
    if (p === "/api/v1/observability") return json(route, obs);
    if (p === "/api/v1/server") return json(route, serverSnapshot);
    if (p === "/api/v1/server/history") return json(route, serverHistory);
    if (p === "/api/v1/server/processes") return json(route, serverProcs);
    if (p === "/api/v1/cron") return json(route, method === "GET" ? cronJobs : { ok: true });
    if (p.startsWith("/api/v1/cron/runs/") && p.endsWith("/log")) return json(route, { log: cronLog });
    if (p.startsWith("/api/v1/cron/") && p.endsWith("/runs")) return json(route, cronRuns);
    if (p.startsWith("/api/v1/cron/") && p.endsWith("/run")) return json(route, { run: 32 });
    if (p.startsWith("/api/v1/cron/")) return json(route, { ok: true });
    if (p === "/api/v1/settings") return json(route, settings);
    if (p === "/api/v1/github") return json(route, { configured: false });
    if (p === "/api/v1/github/repos") return json(route, []);
    if (p === "/api/v1/detect") {
      return json(route, {
        path: "C:\\rkt\\apps\\tienda", framework: "next", pm: "pnpm",
        install: ["pnpm install"], build: ["pnpm run build"],
        start: ["pnpm", "exec", "next", "start", "-H", "127.0.0.1"],
        evidence: ["package.json", "pnpm-lock.yaml"],
      });
    }
    if (p === "/api/v1/fs") {
      const q = url.searchParams.get("path");
      if (!q) {
        return json(route, {
          path: "", parent: "",
          entries: [{ name: "C:\\", path: "C:\\", dir: true }],
          truncated: false,
        });
      }
      return json(route, {
        path: q, parent: "C:\\rkt\\apps",
        entries: [
          { name: "tienda", path: q, dir: true },
          { name: "package.json", path: `${q}\\package.json`, dir: false },
        ],
        truncated: false,
      });
    }
    if (p === "/api/v1/users") return json(route, users);
    if (p === "/api/v1/tokens") return json(route, []);
    if (p === "/api/v1/waf" && method === "GET") {
      return json(route, {
        deny_paths: ["/.env*", "/.git/*", "/wp-*", "/*.php"],
        deny_methods: ["TRACE", "TRACK", "CONNECT"],
        banned: [
          { ip: "45.155.204.66", hits: 23, until: new Date(Date.now() + 8 * 60000).toISOString() },
          { ip: "185.220.101.4", hits: 41, until: new Date(Date.now() + 3 * 60000).toISOString() },
        ],
        blocked: [
          { at: new Date(Date.now() - 4 * 60000).toISOString(), ip: "45.155.204.66", host: "tienda.ejemplo.com", method: "GET", path: "/.env", rule: "/.env*" },
          { at: new Date(Date.now() - 11 * 60000).toISOString(), ip: "45.155.204.66", host: "api.ejemplo.com", method: "TRACE", path: "/", rule: "método TRACE" },
        ],
      });
    }
    if (p === "/api/v1/waf/bans" && method === "DELETE") return json(route, { ok: "true" });
    if (p === "/api/v1/waf") return json(route, { deny_paths: [], deny_methods: [] });
    if (p === "/api/v1/audit") return json(route, audit);
    if (p.startsWith("/api/v1/users/") && p.endsWith("/sessions")) {
      return json(route, {
        current: "sess-current",
        sessions: [
          { id: "sess-current", user: "alex", created_at: iso(2), expires_at: new Date(Date.now() + 30 * 86400000).toISOString() },
          { id: "sess-old", user: "alex", created_at: iso(60 * 24 * 9), expires_at: new Date(Date.now() + 12 * 86400000).toISOString() },
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
    if (p === "/api/v1/fleet/stream") {
      const frames = [
        ["snapshot", { seq: 0, topic: "status", data: { version: "0.6.6", routes: routes.length, apps, http_port: "80", https_port: "443" } }],
        ["snapshot", { seq: 0, topic: "deployments", data: deps }],
        ["snapshot", { seq: 0, topic: "events", data: evs }],
        ["snapshot", { seq: 0, topic: "metas", data: metas }],
        ["hello", { seq: 1, resync: true }],
        ["ping", { t: new Date().toISOString() }],
      ];
      const body = frames.map(([e, d]) => `event: ${e}\ndata: ${JSON.stringify(d)}\n\n`).join("");
      return route.fulfill({ status: 200, contentType: "text/event-stream", body });
    }
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
    if ((await page.locator("h1").count()) === 0) errors.push(`[smoke] ${name}: sin h1`);
    await page.screenshot({ path: join(OUT, `${name}.png`), fullPage: opts.fullPage ?? true });
    console.log("ok", name);
  };

  await shot("overview", "overview");
  await shot("deployments", "deployments");
  await shot("waf", "waf");
  await shot("ci", "ci");
  await shot("logs", "logs");
  await shot("cron", "cron");
  await shot("routes", "routes");
  await shot("proxy", "proxy");
  await shot("analytics", "analytics");
  await shot("observability", "observability");
  await shot("server", "server");
  await shot("alerts", "alerts");
  await shot("environment", "environment");
  await shot("settings", "settings");
  await shot("users", "users");
  for (const tab of ["overview", "deploys", "logs", "analytics", "env", "settings"]) {
    await shot(`app-${tab}`, `app/tienda/${tab}`);
  }
  await shot("app-external", "app/vista-previa/overview");

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
  await page.keyboard.press("Escape");
  await page.waitForTimeout(300);

  // drawer de un deploy en curso (SSE en vivo + checklist de pasos)
  await page.locator('[role="row"]').filter({ hasText: "#125" }).click();
  await page.waitForTimeout(1500);
  await page.screenshot({ path: join(OUT, "deploy-running.png"), fullPage: false });
  console.log("ok deploy-running");
  await page.keyboard.press("Escape");
  await page.waitForTimeout(300);

  // drawer de un deploy fallido (reintento + reversión)
  await page.goto(`${BASE}/deployments`, { waitUntil: "networkidle" });
  await page.waitForTimeout(300);
  await page.locator('[role="row"]').filter({ hasText: "#139" }).click();
  await page.getByText("DETALLE DEL DESPLIEGUE").waitFor({ timeout: 10000 });
  await page.waitForTimeout(800);
  await page.screenshot({ path: join(OUT, "deploy-failed.png"), fullPage: false });
  console.log("ok deploy-failed");

  // confirmación de reversión en dos pasos (dispatch directo: el SSE del
  // mock reconecta en bucle y el click de ratón cae en re-renders)
  await page.getByRole("button", { name: "Revertir" }).dispatchEvent("click");
  await page.waitForTimeout(400);
  await page.screenshot({ path: join(OUT, "deploy-rollback-confirm.png"), fullPage: false });
  console.log("ok deploy-rollback-confirm");
  await page.keyboard.press("Escape");
  await page.waitForTimeout(300);

  // menú de acciones de la ficha de app (redesplegar/reiniciar/revertir)
  await page.goto(`${BASE}/app/tienda/overview`, { waitUntil: "networkidle" });
  await page.waitForTimeout(500);
  await page.getByRole("button", { name: "acciones de tienda" }).click();
  await page.waitForTimeout(400);
  await page.screenshot({ path: join(OUT, "app-actions-menu.png"), fullPage: false });
  console.log("ok app-actions-menu");
  await page.keyboard.press("Escape");
  await page.waitForTimeout(300);

  // revisión de creación con plan detectado (icono del stack)
  await page.goto(`${BASE}/apps/new`, { waitUntil: "networkidle" });
  await page.waitForTimeout(300);
  await page.getByText("Directorio local", { exact: true }).click();
  await page.getByRole("button", { name: "Continuar" }).click();
  await page.getByPlaceholder("C:\\proyectos\\mi-app").fill("C:\\rkt\\apps\\tienda");
  await page.getByRole("button", { name: "Continuar" }).click();
  await page.waitForTimeout(800);
  await page.screenshot({ path: join(OUT, "app-create-review.png"), fullPage: true });
  console.log("ok app-create-review");

  // seguimiento en vivo tras crear (checklist + log del build)
  await page.getByRole("button", { name: "Crear aplicación" }).click();
  await page.waitForTimeout(2500);
  await page.screenshot({ path: join(OUT, "app-create-track.png"), fullPage: true });
  console.log("ok app-create-track");

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
    ["mobile-waf", "waf"],
    ["mobile-app-overview", "app/tienda/overview"],
    ["mobile-proxy", "proxy"],
    ["mobile-logs", "logs"],
    ["mobile-server", "server"],
    ["mobile-cron", "cron"],
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
  for (const [name, route] of [
    ["deployments", "deployments"],
    ["waf", "waf"],
    ["logs", "logs"],
    ["cron", "cron"],
    ["routes", "routes"],
    ["observability", "observability"],
    ["server", "server"],
    ["alerts", "alerts"],
    ["environment", "environment"],
    ["settings", "settings"],
    ["users", "users"],
  ]) {
    await wpage.goto(`${BASE}/${route}`, { waitUntil: "networkidle" });
    await wpage.waitForTimeout(400);
    await wpage.screenshot({ path: join(OUT, `${name}-light.png`), fullPage: true });
    console.log("ok", `${name}-light`);
  }
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
