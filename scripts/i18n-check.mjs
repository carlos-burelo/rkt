// Guard de i18n: toda clave usada con t()/tn() debe existir en el diccionario
// es. tn() acepta claves plurales (.one/.other) sin entrada base.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const src = path.join(here, "..", "web", "src");
const messagesDir = path.join(src, "shared", "i18n", "messages", "es");

function walk(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (full === path.join(src, "shared", "i18n", "messages")) continue;
      walk(full, out);
    } else if (/\.(ts|tsx)$/.test(entry.name)) {
      out.push(full);
    }
  }
  return out;
}

const used = new Map();
for (const file of walk(src)) {
  const text = fs.readFileSync(file, "utf8");
  for (const m of text.matchAll(/\bt[n]?\(\s*"([^"]+)"/g)) {
    used.set(m[1], path.relative(src, file));
  }
}

const defs = new Map();
for (const file of fs.readdirSync(messagesDir)) {
  if (!file.endsWith(".ts")) continue;
  const text = fs.readFileSync(path.join(messagesDir, file), "utf8");
  for (const m of text.matchAll(/"([^"]+)"\s*:/g)) {
    const key = m[1];
    if (defs.has(key)) {
      console.error(`clave duplicada: ${key} (${defs.get(key)} y ${file})`);
      process.exitCode = 1;
    }
    defs.set(key, file);
  }
}

const missing = [];
for (const [key, file] of used) {
  const ok =
    defs.has(key) ||
    defs.has(`${key}.zero`) ||
    defs.has(`${key}.one`) ||
    defs.has(`${key}.other`);
  if (!ok) missing.push(`${key} (${file})`);
}

if (missing.length) {
  console.error(`claves sin definir (${missing.length}):`);
  for (const m of missing) console.error(`  - ${m}`);
  process.exitCode = 1;
}

if (!process.exitCode) {
  console.log(`i18n ok: ${used.size} claves usadas, ${defs.size} definidas`);
}
