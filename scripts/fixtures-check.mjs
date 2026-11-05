// Guard de fixtures: nombres regionales, marcas ajenas y voseo.
import { readFileSync } from "node:fs";

const files = ["scripts/shots/mock.mjs", "scripts/shots/fixtures.mjs"];
const forbidden = [
  /\bxavi\b/i,
  /\bmira\b/i,
  /\btomas\b/i,
  /\bcoatl\b/i,
  /\bvos\b/i,
  /\bpod[eé]s\b/i,
  /\bten[eé]s\b/i,
  /\bac[aá]\b/i,
  /\bvarado\b/i,
];
let bad = [];
for (const file of files) {
  const text = readFileSync(file, "utf8");
  for (const rx of forbidden) {
    const m = text.match(rx);
    if (m) bad.push(`${file}: ${m[0]}`);
  }
}
if (bad.length) {
  console.error("fixtures con nombres/copy no neutros:");
  for (const b of bad) console.error(" -", b);
  process.exit(1);
}
console.log("fixtures ok");
