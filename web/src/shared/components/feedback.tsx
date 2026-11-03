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
      className="rounded-[1px] p-0.5 text-muted-foreground/60 transition-colors hover:bg-accent hover:text-foreground"
    >
      <Copy className="h-3 w-3" />
    </button>
  );
}

// Skeleton filas de carga (boot, no vacío).
export function Skeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="border border-border">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex h-12 items-center gap-3 border-b border-border bg-card/35 px-3 last:border-0">
          <SkeletonPrimitive className="h-1.5 w-1.5 rounded-[1px]" />
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
  labels,
  height = 64,
  tone,
  showValues = false,
}: {
  data: number[];
  labels?: string[];
  height?: number;
  tone?: (v: number, max: number) => string;
  showValues?: boolean;
}) {
  const max = Math.max(1, ...data);
  const top = showValues ? 16 : 0;
  return (
    <figure className="relative">
      <div className="relative" style={{ height: height + top }}>
        <div
          className="pointer-events-none absolute inset-x-0 flex flex-col justify-between"
          style={{ top, bottom: 0 }}
          aria-hidden
        >
          {[0, 1, 2, 3].map((i) => <span key={i} className="h-px w-full bg-border/45" />)}
        </div>
        {showValues && (
          <div className="absolute inset-x-0 top-0 flex gap-1.5" aria-hidden>
            {data.map((v, i) => (
              <span key={i} className="tnum min-w-0 flex-1 truncate text-center font-mono text-[8px] leading-4 text-muted-foreground/75">
                {v}
              </span>
            ))}
          </div>
        )}
        <div
          className="absolute inset-x-0 bottom-0 flex items-end gap-1.5 border-b border-border pb-px"
          style={{ height }}
          role="img"
          aria-label={`serie de ${data.length} puntos`}
        >
          {data.map((v, i) => (
            <div
              key={i}
              className={cn(
                "min-w-px flex-1 border-t border-foreground/15 transition-colors",
                tone ? tone(v, max) : v > 0 ? "bg-muted-foreground/35 hover:bg-signal/65" : "bg-muted/60",
              )}
              style={{ height: `${Math.max(2, (v / max) * 100)}%` }}
              title={`${labels?.[i] ? `${labels[i]} · ` : ""}${v}`}
            />
          ))}
        </div>
      </div>
      {labels && (
        <div className="mt-1.5 flex gap-1.5" aria-hidden>
          {labels.map((l, i) => (
            <span key={i} className="min-w-0 flex-1 truncate text-center font-mono text-[8px] tracking-[0.04em] text-muted-foreground/70">
              {l}
            </span>
          ))}
        </div>
      )}
    </figure>
  );
}
