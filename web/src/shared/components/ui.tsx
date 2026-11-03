export function Dot({ status }: { status: string }) {
  const ok = status === "ok" || status === "running";
  const run = status === "running";
  return (
    <span
      className={`inline-block h-2 w-2 rounded-full ${ok ? "bg-emerald-400" : "bg-red-400"} ${run ? "animate-pulse" : ""}`}
      title={status}
    />
  );
}

export function Err({ msg }: { msg: string }) {
  if (!msg) return null;
  return <p className="mb-4 rounded-md border border-red-900 bg-red-950/50 px-3 py-2 text-xs text-red-300">{msg}</p>;
}

// Glyphs propios (geométricos texto, sin copiar iconos ajenos).
export const Glyph: Record<string, string> = {
  overview: "▦",
  proxy: "⬡",
  deployments: "⬢",
  ci: "▶",
  logs: "☰",
  analytics: "◣",
  observability: "◉",
  alerts: "!",
  routes: "⇄",
  environment: "≡",
  settings: "⚙",
  users: "○",
  app: "▲",
};

export function Avatar({ name }: { name: string }) {
  const ch = (name.trim()[0] ?? "?").toUpperCase();
  return (
    <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-zinc-700 text-[11px] font-semibold text-zinc-100">
      {ch}
    </span>
  );
}
