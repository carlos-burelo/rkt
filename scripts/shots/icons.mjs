// Genera favicon e iconos PWA desde web/public/icons/mark*.svg.
// Uso: node scripts/shots/icons.mjs
import { chromium } from "playwright";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const out = join(root, "web", "public", "icons");
const mark = readFileSync(join(out, "mark.svg"), "utf8");
const small = readFileSync(join(out, "mark-small.svg"), "utf8");
const maskable = readFileSync(join(out, "mark-maskable.svg"), "utf8");

const targets = [
  ["favicon-16.png", 16, small],
  ["favicon-32.png", 32, mark],
  ["apple-touch-icon.png", 180, mark],
  ["icon-192.png", 192, mark],
  ["icon-512.png", 512, mark],
  ["icon-maskable-512.png", 512, maskable],
];

const browser = await chromium.launch();
try {
  for (const [file, size, svg] of targets) {
    const page = await browser.newPage({
      viewport: { width: size, height: size },
      deviceScaleFactor: 1,
    });
    await page.setContent(
      `<style>html,body{margin:0;background:transparent}svg{display:block;width:${size}px;height:${size}px}</style>${svg}`,
    );
    await page.screenshot({ path: join(out, file), omitBackground: true });
    await page.close();
    console.log(`${file} ${size}x${size}`);
  }
} finally {
  await browser.close();
}
