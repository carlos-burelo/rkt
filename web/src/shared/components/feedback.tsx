import { Skeleton as SkeletonPrimitive } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

// CopyButton copia al portapapeles con micro-feedback.
export function CopyButton({ text }: { text: string }) {
  return (
    <button
      onClick={(e) => {
        e.stopPropagation();
        navigator.clipboard?.writeText(text).catch(() => {});
      }}
      title="copiar"
      className="rounded px-1 font-mono text-[11px] text-zinc-600 hover:bg-zinc-800 hover:text-zinc-200"
    >
      ⧉
    </button>
  );
}

// Skeleton filas de carga (boot, no vacío).
export function Skeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="overflow-hidden rounded-lg border border-zinc-800">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-3 last:border-0">
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

export function Bars({ data, height = 64 }: { data: number[]; height?: number }) {
  const max = Math.max(1, ...data);
  return (
    <div className="flex items-end gap-0.5" style={{ height }}>
      {data.map((v, i) => (
        <div
          key={i}
          className={cn("min-w-1 flex-1 rounded-sm", v > 0 ? "bg-zinc-500" : "bg-zinc-800")}
          style={{ height: `${Math.max(6, (v / max) * 100)}%` }}
          title={`${v}`}
        />
      ))}
    </div>
  );
}
