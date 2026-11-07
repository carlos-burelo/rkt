// Formato del monitor del host (puro, testeable).
export function bytes(n: number): string {
  if (!Number.isFinite(n)) return "—";
  if (n >= 1 << 30) return `${(n / (1 << 30)).toFixed(1)} GB`;
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(0)} KB`;
  return `${Math.round(n)} B`;
}

export function rate(n: number): string {
  return `${bytes(n)}/s`;
}

export function uptime(s: number, t: (k: string, v?: Record<string, string | number>) => string): string {
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return t("server.uptime.days", { d, h });
  if (h > 0) return t("server.uptime.hours", { h, m });
  return t("server.uptime.minutes", { m });
}

type Tone = "ok" | "warn" | "bad" | "signal";

export function tone(frac: number | undefined): Tone {
  if (frac === undefined) return "signal";
  if (frac > 0.9) return "bad";
  if (frac > 0.7) return "warn";
  return "ok";
}
