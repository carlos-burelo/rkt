import { Clock, Globe, Lock, LockOpen } from "lucide-react";
import { useI18n } from "../../shared/i18n";
import type { Level, Stream } from "./format";

export const LEVEL_KEY: Record<Level, string> = {
  info: "ops.logs.level.info",
  warning: "ops.logs.level.warning",
  error: "ops.logs.level.error",
};

export const ROW_TONE: Record<Level, string> = {
  info: "text-console-foreground/85",
  warning: "text-console-warn",
  error: "text-console-bad",
};

function shortVal(value: string): string {
  return value.length > 40 ? `${value.slice(0, 40)}…` : value;
}

// Bit compacto con iconografía: candado abierto/cerrado para http/https,
// mundo para host, reloj para duración. Valores vacíos se ocultan
// (ej `https=` sin TLS solo mete ruido). title conserva clave=valor.
export function FieldBit({ fkey, value }: { fkey: string; value: string }) {
  if (value === "") return null;
  if (fkey === "https") {
    return (
      <span title={`https=${value}`} className="inline-flex items-center gap-1 whitespace-nowrap">
        <Lock className="h-3 w-3 shrink-0 opacity-70" aria-label="https" /> {shortVal(value)}
      </span>
    );
  }
  if (fkey === "http") {
    return (
      <span title={`http=${value}`} className="inline-flex items-center gap-1 whitespace-nowrap">
        <LockOpen className="h-3 w-3 shrink-0 opacity-70" aria-label="http" /> {shortVal(value)}
      </span>
    );
  }
  if (fkey === "host") {
    return (
      <span title={`host=${value}`} className="inline-flex items-center gap-1 whitespace-nowrap opacity-80">
        <Globe className="h-3 w-3 shrink-0 opacity-70" aria-hidden /> {shortVal(value)}
      </span>
    );
  }
  if (fkey === "dur" || fkey === "duration" || fkey === "duration_ms" || fkey === "latency" || fkey === "latency_ms") {
    return (
      <span title={`${fkey}=${value}`} className="inline-flex items-center gap-1 whitespace-nowrap opacity-70">
        <Clock className="h-3 w-3 shrink-0 opacity-70" aria-hidden /> {shortVal(value)}
      </span>
    );
  }
  if (fkey === "target") {
    return <span title={`target=${value}`} className="whitespace-nowrap opacity-80">→ {shortVal(value)}</span>;
  }
  if (fkey === "remote" || fkey === "remote_addr" || fkey === "ip") {
    return <span title={`${fkey}=${value}`} className="whitespace-nowrap opacity-70">← {shortVal(value)}</span>;
  }
  if (fkey === "method" || fkey === "m") {
    return <span className="whitespace-nowrap font-semibold">{value.toUpperCase()}</span>;
  }
  if (fkey === "status" || fkey === "status_code" || fkey === "st") {
    return <span className="whitespace-nowrap font-semibold">{value}</span>;
  }
  if (fkey === "path" || fkey === "p" || fkey === "query" || fkey === "q") {
    return <span className="whitespace-nowrap">{shortVal(value)}</span>;
  }
  return (
    <span title={`${fkey}=${value}`} className="whitespace-nowrap opacity-60">
      {fkey}={shortVal(value)}
    </span>
  );
}

export function Highlight({ text, q }: { text: string; q: string }) {
  const needle = q.trim();
  if (needle === "") return <>{text}</>;
  const rx = new RegExp(needle.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "gi");
  const out: React.ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(rx)) {
    const i = m.index ?? 0;
    if (i > last) out.push(text.slice(last, i));
    out.push(
      <mark key={`${i}-${m[0]}`} className="bg-console-warn/25 text-console-warn">
        {m[0]}
      </mark>,
    );
    last = i + m[0].length;
  }
  if (last < text.length) out.push(text.slice(last));
  return <>{out}</>;
}

export function ago(ts?: number): string {
  if (!ts) return "";
  const s = Math.max(0, Math.round((Date.now() - ts) / 1000));
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.round(s / 60)}m`;
  return `${Math.round(s / 3600)}h`;
}
