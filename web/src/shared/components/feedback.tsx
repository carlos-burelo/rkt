import { Copy } from "lucide-react";
import { Skeleton as SkeletonPrimitive } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

// CopyButton copia al portapapeles con micro-feedback.
export function CopyButton({ text, label = "copiar" }: { text: string; label?: string }) {
  return (
    <button
      onClick={(e) => {
        e.stopPropagation();
        navigator.clipboard?.writeText(text).catch(() => {});
      }}
      title={label}
      aria-label={label}
      className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
    >
      <Copy className="h-3.5 w-3.5" />
    </button>
  );
}

// Skeleton filas de carga (boot, no vacío).
export function Skeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="overflow-hidden rounded-lg border">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3 border-b bg-card/40 px-4 py-3 last:border-0">
          <SkeletonPrimitive className="h-2 w-2 rounded-full" />
          <SkeletonPrimitive className="h-3 w-32" />
          <SkeletonPrimitive className="h-3 w-20" />
          <div className="flex-1" />
          <SkeletonPrimitive className="h-3 w-16" />
        </div>
      ))}
    </div>
  );
}

export function Bars({
  data,
  height = 64,
  tone,
}: {
  data: number[];
  height?: number;
  tone?: (v: number, max: number) => string;
}) {
  const max = Math.max(1, ...data);
  return (
    <div className="flex items-end gap-1 border-b border-border/50 pb-px" style={{ height }} role="img" aria-label={`serie de ${data.length} puntos`}>
      {data.map((v, i) => (
        <div
          key={i}
          className={cn("min-w-[3px] flex-1 rounded-[2px]", tone ? tone(v, max) : v > 0 ? "bg-muted-foreground/50" : "bg-muted")}
          style={{ height: `${Math.max(6, (v / max) * 100)}%` }}
          title={`${v}`}
        />
      ))}
    </div>
  );
}
