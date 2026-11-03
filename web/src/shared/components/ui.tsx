import { Avatar as AvatarPrimitive, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

export type FleetState = "running" | "degraded" | "failed" | "stopped" | "ok";

export function stateOf(a: { running: boolean; failed?: boolean; restarts?: number }): FleetState {
  if (a.failed) return "failed";
  if (a.running) return "running";
  return "stopped";
}

const DOT: Record<string, string> = {
  ok: "bg-emerald-400",
  running: "bg-emerald-400 animate-pulse",
  degraded: "bg-amber-400",
  failed: "bg-red-400",
  stopped: "bg-muted-foreground/60",
};

export function Dot({ status }: { status: string }) {
  const key = status === "ok" ? "ok" : status === "running" ? "running" : status;
  return (
    <span
      className={cn("inline-block h-2 w-2 shrink-0 rounded-full", DOT[key] ?? DOT.stopped)}
      title={status}
      aria-label={`estado ${status}`}
    />
  );
}

const BADGE_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  ok: "secondary",
  running: "secondary",
  degraded: "outline",
  failed: "destructive",
  stopped: "outline",
};

export function StatusBadge({ status, className }: { status: string; className?: string }) {
  return (
    <Badge
      variant={BADGE_VARIANT[status] ?? "outline"}
      className={cn("tnum font-mono text-[11px] font-normal", className)}
    >
      <Dot status={status} />
      <span className="ml-1.5">{status}</span>
    </Badge>
  );
}

export function Err({ msg }: { msg: string }) {
  if (!msg) return null;
  return (
    <p className="mb-4 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
      {msg}
    </p>
  );
}

const GLYPH_TONES = [
  "bg-signal/15 text-signal",
  "bg-emerald-500/15 text-emerald-300",
  "bg-sky-500/15 text-sky-300",
  "bg-amber-500/15 text-amber-300",
  "bg-violet-500/15 text-violet-300",
];

export function Avatar({ name }: { name: string }) {
  const ch = (name.trim()[0] ?? "?").toUpperCase();
  let h = 0;
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return (
    <AvatarPrimitive className="h-6 w-6 text-[11px]">
      <AvatarFallback className={cn("font-semibold", GLYPH_TONES[h % GLYPH_TONES.length])}>
        {ch}
      </AvatarFallback>
    </AvatarPrimitive>
  );
}
