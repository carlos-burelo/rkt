// Reglas puras de la ficha de app (testeables sin DOM).
export function canSaveApp(command: string, external: boolean, staticDir?: string): boolean {
  if (external || staticDir) return true;
  return command.trim() !== "";
}

// StackKind identidad visual de la tecnología (canónico; lo usa TechIcon).
// "unknown" = sin icono (no adivinar en la UI).
export type StackKind =
  | "next" | "vite" | "astro" | "sveltekit" | "nuxt" | "remix" | "solidstart"
  | "node" | "go" | "rust" | "java" | "python" | "static" | "external" | "unknown";

// stackOfApp heurística display-only sobre command/args/build: el manifiesto
// no guarda el plan, así que la UI lo deriva. Orden: frameworks concretos
// antes que runtimes genéricos (pnpm/node).
export function stackOfApp(meta: {
  command?: string; args?: string[]; static?: string; external?: boolean;
  deploy?: { build?: string[] } | null;
}): StackKind {
  if (meta.external) return "external";
  if (meta.static) return "static";
  const hay = [meta.command ?? "", ...(meta.args ?? []), ...((meta.deploy?.build) ?? [])]
    .join(" ").toLowerCase();
  const tok = new Set(hay.split(/[^a-z0-9]+/).filter(Boolean));
  const has = (...ws: string[]) => ws.some((w) => tok.has(w));
  if (has("next")) return "next";
  if (has("vite")) return "vite";
  if (has("astro")) return "astro";
  if (has("svelte", "sveltekit")) return "sveltekit";
  if (has("nuxt")) return "nuxt";
  if (has("remix")) return "remix";
  if (has("solid", "solidstart")) return "solidstart";
  if (has("python")) return "python";
  if (has("cargo") || hay.includes("target/release") || hay.includes("target\\release")) return "rust";
  if (has("mvn", "gradle", "gradlew") || hay.includes(".jar") || hay.includes("spring-boot")) return "java";
  if (hay.includes("go build") || /(^|\/)go(\.exe)?$/.test((meta.command ?? "").toLowerCase().replace(/\\/g, "/"))) return "go";
  if (has("pnpm", "npm", "node", "nodejs", "yarn", "bun", "deno", "serve", "tsx")) return "node";
  return "unknown";
}

export function normalizeDomain(raw: string): string {
  return raw
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, "")
    .replace(/\/.*$/, "")
    .replace(/\.$/, "");
}
