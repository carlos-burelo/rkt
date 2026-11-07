import { Link } from "react-router";
import { ArrowRight } from "lucide-react";
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

// Tira de estado: una línea de 1px en el canto de la fila.
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

// Glyph de app: cuadrado con inicial, no avatar circular.
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

// Marca propia: cohete sobre halo, misma fuente que el favicon.
export function BrandMark({ className }: { className?: string }) {
  return (
    <img
      src="/icons/mark.svg"
      alt=""
      aria-hidden
      draggable={false}
      className={className}
    />
  );
}

export function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="inline-flex h-4 min-w-4 items-center justify-center rounded-[1px] border border-border bg-secondary/50 px-1 font-mono text-[9px] text-muted-foreground">
      {children}
    </kbd>
  );
}

const TEXT_TONE = {
  ok: "text-ok",
  warn: "text-warn",
  bad: "text-bad",
  muted: "text-muted-foreground",
  info: "text-info",
  signal: "text-signal",
} as const;

// MicroLink: enlace micro tipográfico, interno (to) o externo (href).
// Unifica los "ver más →" sueltos de cada vista.
export function MicroLink({
  to,
  href,
  children,
  arrow = true,
  className,
}: {
  to?: string;
  href?: string;
  children: React.ReactNode;
  arrow?: boolean;
  className?: string;
}) {
  const cls = cn(
    "inline-flex items-center gap-1 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:text-foreground",
    className,
  );
  const inner = (
    <>
      {children}
      {arrow && <ArrowRight className="h-3 w-3" aria-hidden />}
    </>
  );
  if (to) {
    return (
      <Link to={to} className={cls}>
        {inner}
      </Link>
    );
  }
  return (
    <a href={href} target="_blank" rel="noreferrer" className={cls}>
      {inner}
    </a>
  );
}

// StatusText: etiqueta diminuta coloreada por tono (canónico; antes cada
// vista repetía el ternario text-ok/text-warn/text-bad).
export function StatusText({
  tone = "muted",
  children,
  className,
}: {
  tone?: keyof typeof TEXT_TONE;
  children: React.ReactNode;
  className?: string;
}) {
  return <span className={cn("font-mono text-[9px] uppercase", TEXT_TONE[tone], className)}>{children}</span>;
}
