// Genera favicon e iconos PWA desde web/public/icons/mark.svg.
// Uso: node scripts/shots/icons.mjs
import { chromium } from "playwright";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const out = join(root, "web", "public", "icons");
const mark = readFileSync(join(out, "mark.svg"), "utf8");
// Maskable: misma marca, fondo lleno y escala dentro de la zona segura.
const maskable = mark
  .replace(/(<svg[^>]*>)/, `$1<rect width="512" height="512" fill="#0a0a0c"/>`)
  .replace(/scale\([\d.]+\)/, "scale(1.02)");

const targets = [
  ["favicon-16.png", 16, mark],
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
