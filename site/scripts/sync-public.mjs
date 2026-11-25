// Copia byte-exacta scripts/*.ps1 del repo a site/public/ (generado, no se comitea).
// Fuente canónica: <repo>/scripts/. El site solo los publica en /install.ps1 y /nuke.ps1.
import { copyFileSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const out = join(root, "site", "public");
mkdirSync(out, { recursive: true });
for (const f of ["install.ps1", "nuke.ps1"]) {
  copyFileSync(join(root, "scripts", f), join(out, f));
  console.log(`site/public/${f} <- scripts/${f}`);
}
