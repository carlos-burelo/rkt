import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { stateLabel } from "../lib/labels";
import { useI18n } from "../i18n";

export type FleetState = "running" | "degraded" | "failed" | "stopped" | "ok";

export function stateOf(a: { running: boolean; failed?: boolean; restarts?: number; http_ok?: boolean }): FleetState {
  if (a.failed) return "failed";
  if (a.running) return a.http_ok === false ? "degraded" : "running";
  return "stopped";
}

const DOT: Record<string, string> = {
  ok: "bg-ok",
  running: "bg-ok",
  degraded: "rotate-45 bg-warn",
  failed: "rounded-full bg-bad",
  stopped: "border border-muted-foreground/50 bg-transparent",
};

export function Dot({ status, className, pulse = false }: { status: string; className?: string; pulse?: boolean }) {
  const { t } = useI18n();
  const key = status === "ok" ? "ok" : status === "running" ? "running" : status;
  return (
    <span
      className={cn(
        "inline-block h-2 w-2 shrink-0 rounded-[1px]",
        DOT[key] ?? DOT.stopped,
        pulse && "animate-status-pulse",
        className,
      )}
      title={stateLabel(status)}
      aria-label={t("shared.dot.aria", { state: stateLabel(status) })}
    />
  );
}

const STATUS_TONE: Record<string, string> = {
  ok: "border-ok/30 bg-ok/10 text-ok",
  running: "border-ok/30 bg-ok/10 text-ok",
  degraded: "border-warn/30 bg-warn/10 text-warn",
  failed: "border-bad/30 bg-bad/10 text-bad",
  stopped: "border-border bg-transparent text-muted-foreground",
};

export function StatusBadge({ status, className }: { status: string; className?: string }) {
  return (
    <Badge
      variant="outline"
      className={cn("tnum", STATUS_TONE[status] ?? STATUS_TONE.stopped, className)}
    >
      {stateLabel(status)}
    </Badge>
  );
}

export function Err({ msg }: { msg: string }) {
  if (!msg) return null;
  return (
    <p role="alert" className="mb-4 flex items-start gap-2 border border-bad/30 bg-bad/8 px-3 py-2 font-mono text-[11px] leading-relaxed text-bad">
      <span aria-hidden className="mt-px shrink-0">!</span>
      <span>{msg}</span>
    </p>
  );
}

// Tira de manifiesto por estado: una línea de 1px en el canto de la fila.
// Solida = sana, discontinua = degradada, rota = fallida, tenue = detenida.
export function StateRail({ status }: { status: string }) {
  const cls =
    status === "failed" || status === "bad"
      ? "rail-bad"
      : status === "running" || status === "ok"
        ? "rail-ok"
        : status === "degraded" || status === "warning"
          ? "rail-warn"
          : "rail-idle";
  return <span aria-hidden className={`manifest-rail ${cls}`} />;
}

// Glyph de unidad: cuadrado con inicial, no avatar circular.
const GLYPH_TONES = [
  "border-border bg-secondary/60 text-muted-foreground",
  "border-border bg-secondary/60 text-muted-foreground",
  "border-border bg-secondary/60 text-muted-foreground",
  "border-border bg-secondary/60 text-muted-foreground",
];

export function Avatar({ name, className }: { name: string; className?: string }) {
  const ch = (name.trim()[0] ?? "?").toUpperCase();
  let h = 0;
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return (
    <span
      className={cn(
        "inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-[1px] border font-mono text-[10px] font-medium uppercase",
        GLYPH_TONES[h % GLYPH_TONES.length],
        className,
      )}
      aria-hidden
    >
      {ch}
    </span>
  );
}

// Marca propia: esquina de contenedor / folio, dibujada, no copiada.
export function BrandMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden className={className}>
      <path d="M3.5 3.5h12.8l4.2 4.2v12.8H3.5V3.5Z" stroke="currentColor" strokeWidth="1.4" />
      <path d="M16.3 3.5v4.2h4.2" stroke="currentColor" strokeWidth="1.4" />
      <path d="M3.5 12h17M8 16h8" stroke="currentColor" strokeWidth="1" opacity=".55" />
      <path d="M6.5 6.5h2.2v2.2H6.5zM6.5 15.3h2.2v2.2H6.5z" fill="currentColor" opacity=".8" />
    </svg>
  );
}

export function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="inline-flex h-4 min-w-4 items-center justify-center rounded-[1px] border border-border bg-secondary/50 px-1 font-mono text-[9px] text-muted-foreground">
      {children}
    </kbd>
  );
}
