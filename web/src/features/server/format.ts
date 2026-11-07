// Formato del monitor del host (puro, testeable).
// bytes vive en el hogar canónico (shared/lib/format); aquí solo el
// resguardo NaN/Infinity propio del monitor.
import { bytes as baseBytes } from "../../shared/lib/format";

export function bytes(n: number): string {
  if (!Number.isFinite(n)) return "—";
  return baseBytes(n);
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
