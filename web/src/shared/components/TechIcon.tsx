import { useTheme } from "../theme";
import type { StackKind } from "../../features/apps/model";

// Iconos de stack (logos de https://svgl.app, estilo original intacto).
// La pastilla clara da contraste en ambos temas sin tocar el SVG.
const FILES: Record<string, string> = {
  next: "next.svg",
  vite: "vite.svg",
  astro: "astro",
  sveltekit: "svelte.svg",
  nuxt: "nuxt.svg",
  remix: "remix",
  solidstart: "solidjs.svg",
  node: "node.svg",
  go: "go.svg",
  rust: "rust.svg",
  java: "java.svg",
  python: "python.svg",
  static: "static.svg",
};

const LABELS: Record<string, string> = {
  next: "Next.js",
  vite: "Vite",
  astro: "Astro",
  sveltekit: "SvelteKit",
  nuxt: "Nuxt",
  remix: "Remix",
  solidstart: "SolidStart",
  node: "Node.js",
  go: "Go",
  rust: "Rust",
  java: "Java",
  python: "Python",
  static: "Static",
};

function fileFor(kind: string, resolved: "light" | "dark"): string | null {
  const base = FILES[kind];
  if (!base) return null;
  if (base === "astro" || base === "remix") {
    return `${base}-${resolved}.svg`;
  }
  return base;
}

export function TechIcon({ kind, size = 20 }: { kind: StackKind | string; size?: number }) {
  const { resolved } = useTheme();
  const file = fileFor(kind, resolved);
  if (!file) return null;
  return (
    <span
      role="img"
      aria-label={LABELS[kind] ?? kind}
      title={LABELS[kind] ?? kind}
      className="flex shrink-0 items-center justify-center rounded-[3px] bg-white p-[3px] ring-1 ring-black/10"
      style={{ width: size, height: size }}
    >
      <img src={`/icons/tech/${file}`} alt="" draggable={false} className="h-full w-full object-contain" />
    </span>
  );
}
