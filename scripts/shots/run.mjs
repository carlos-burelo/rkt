// Capturas del dashboard a docs/shots/. Sin dependencias del repo:
// login por API + Chromium headless + cookie de sesión.
// Uso: RKT_USER=.. RKT_PASS=.. [RKT_URL=..] pnpm shots
import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const OUT = join(ROOT, "docs", "shots");
const BASE = (process.env.RKT_URL ?? "http://127.0.0.1:8090").replace(/\/$/, "");
const USER = process.env.RKT_USER ?? "";
const PASS = process.env.RKT_PASS ?? "";

if (!USER || !PASS) {
  console.error("falta RKT_USER / RKT_PASS");
  process.exit(2);
}

const main = async () => {
  // login por API (misma sesión cookie que el browser)
  const loginRes = await fetch(BASE + "/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user: USER, password: PASS }),
  });
  if (!loginRes.ok) throw new Error("login fallido (revisa user/pass)");
  const setCookie = loginRes.headers.get("set-cookie") ?? "";
  const token = /rkt_token=([^;]+)/.exec(setCookie)?.[1];
  if (!token) throw new Error("login sin cookie");

  const browser = await chromium.launch();
  const ctx = await browser.newContext({
    viewport: { width: 1600, height: 900 },
    baseURL: BASE + "/dashboard/",
  });
  await ctx.addCookies([
    { name: "rkt_token", value: token, domain: "127.0.0.1", path: "/" },
  ]);
  const page = await ctx.newPage();
  page.on("console", (m) => {
    if (m.type() === "error") console.log(`[consola:${page.url()}]`, m.text());
  });
  page.on("pageerror", (e) => console.log(`[pageerror:${page.url()}]`, e.message));

  mkdirSync(OUT, { recursive: true });
  const shot = async (name, route) => {
    await page.goto(route, { waitUntil: "networkidle" });
    await page.waitForTimeout(2500); // deja asentar polls/skeletons
    await page.screenshot({ path: join(OUT, `${name}.png`), fullPage: true });
    console.log("ok", name);
  };

  const apps = await fetch(BASE + "/api/v1/apps", {
    headers: { Cookie: `rkt_token=${token}` },
  }).then((r) => (r.ok ? r.json() : []));

  const routes = [
    ["overview", "overview"],
    ["proxy", "proxy"],
    ["deployments", "deployments"],
    ["ci", "ci"],
    ["logs", "logs"],
    ["analytics", "analytics"],
    ["observability", "observability"],
    ["alerts", "alerts"],
    ["routes", "routes"],
    ["environment", "environment"],
    ["settings", "settings"],
  ];
  for (const [name, route] of routes) {
    await shot(name, route);
  }

  // users solo admin (el backend manda 403 si no)
  const meRole = await fetch(BASE + "/api/v1/auth/me", {
    headers: { Cookie: `rkt_token=${token}` },
  })
    .then((r) => (r.ok ? r.json() : null))
    .then((j) => j?.role)
    .catch(() => null);
  if (meRole === "admin") {
    await shot("users", "users");
  } else {
    console.log("skip users (no admin)");
  }

  // tabs de la primera app (si hay)
  if (apps.length > 0) {
    const app = apps[0].name;
    for (const tab of ["overview", "deploys", "logs", "analytics", "env", "settings"]) {
      await shot(`app-${tab}`, `app/${app}/${tab}`);
    }
  } else {
    console.log("skip app/* (sin apps)");
  }

  await browser.close();
  console.log("listo en docs/shots/");
};

main().catch((e) => {
  console.error("shots:", e.message);
  process.exit(1);
});
