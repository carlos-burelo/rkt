import { useEffect } from "react";

export interface Toast {
  id: number;
  kind: "ok" | "err";
  msg: string;
}

// Toasts pila auto-dismiss 4s. Sin deps.
export function ToastStack({ toasts, dismiss }: { toasts: Toast[]; dismiss: (id: number) => void }) {
  useEffect(() => {
    if (toasts.length === 0) return;
    const t = setTimeout(() => dismiss(toasts[0].id), 4000);
    return () => clearTimeout(t);
  }, [toasts, dismiss]);
  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-72 flex-col gap-2">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`pointer-events-auto rounded-lg border px-3 py-2 text-xs shadow-xl ${
            t.kind === "ok"
              ? "border-emerald-900 bg-emerald-950/90 text-emerald-200"
              : "border-red-900 bg-red-950/90 text-red-200"
          }`}
        >
          {t.msg}
        </div>
      ))}
    </div>
  );
}

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
          <div className="h-2 w-2 animate-pulse rounded-full bg-zinc-700" />
          <div className="h-3 w-32 animate-pulse rounded bg-zinc-800" />
          <div className="h-3 w-20 animate-pulse rounded bg-zinc-800" />
          <div className="flex-1" />
          <div className="h-3 w-16 animate-pulse rounded bg-zinc-800" />
        </div>
      ))}
    </div>
  );
}
