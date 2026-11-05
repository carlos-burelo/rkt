import { chromium } from "playwright";

const apps = [{ name: "checkout", running: true, restarts: 0, failed: false }];
const metas = [{ name: "checkout", command: "node server.js", args: [], dir: "C:/rkt/apps/checkout", restart_delay: "2s", min_uptime: "10s", memory_mb: 512, port: "3001", domains: ["checkout.localhost"], deploy: { repo: "https://github.com/xavi/checkout", ref: "main", build: [], auto: true } }];
const routes = [{ hosts: ["checkout.localhost"], target: "http://127.0.0.1:3001" }];
const analytics = { total: 7105, bytes: 428_000_000, by_class: { "2xx": 6832, "3xx": 141, "4xx": 96, "5xx": 36 }, by_day: { "2026-10-27": 812 }, by_path: [], by_host: [], by_ref: [], p50: "42ms", p95: "188ms" };

const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 });
const page = await ctx.newPage();
await page.route("**/api/v1/**", (route) => {
  const p = new URL(route.request().url()).pathname;
  const body =
    p === "/api/v1/auth/status" ? { setup_required: false } :
    p === "/api/v1/auth/me" ? { user: "xavi", role: "admin" } :
    p === "/api/v1/status" ? { version: "0.4.2", routes: 1, apps } :
    p === "/api/v1/deployments" ? [] :
    p === "/api/v1/events" ? [] :
    p === "/api/v1/apps" ? metas :
    p.endsWith("/env") ? { A: "1" } :
    p === "/api/v1/proxy" ? routes.map((r) => ({ ...r, ok: true, tls: ["2026-12-14"] })) :
    p === "/api/v1/routes" ? routes :
    p === "/api/v1/analytics" ? analytics :
    {};
  route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
});
await page.goto("http://localhost:5173/app/checkout/overview", { waitUntil: "networkidle" });
const section = page.locator("section").filter({ hasText: "Tráfico 7d" }).first();
await section.screenshot({ path: "docs/shots/debug-trafico.png" });
console.log("box", await section.boundingBox());
await browser.close();
