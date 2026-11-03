import { useState } from "react";
import { CopyButton } from "./feedback";
import { rel } from "../lib/time";

// PageHeader patrón Vercel: título grande + descripción + acciones.
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
}) {
  return (
    <div className="mb-5 flex flex-wrap items-end gap-3">
      <div className="min-w-0 flex-1">
        <h2 className="text-xl font-semibold tracking-tight">{title}</h2>
        {description && <p className="mt-0.5 text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  );
}

// Section tarjeta con encabezado (patrón settings Vercel).
export function Section({
  title,
  description,
  actions,
  children,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className="rounded-xl border bg-card text-card-foreground shadow">
      <div className="flex items-start gap-3 border-b px-5 py-4">
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium">{title}</h3>
          {description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
        </div>
        {actions}
      </div>
      <div className="p-5">{children}</div>
    </section>
  );
}

// Time relativo con absoluto en tooltip.
export function Time({ iso, className }: { iso: string; className?: string }) {
  if (!iso) return <span className={className}>—</span>;
  const d = new Date(iso);
  const abs = Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
  return (
    <span className={className} title={abs}>
      {rel(iso)}
    </span>
  );
}

// EnvSecret valor enmascarado con revelar + copiar (higiene secretos).
export function EnvSecret({ value }: { value: string }) {
  const [show, setShow] = useState(false);
  return (
    <span className="flex min-w-0 flex-1 items-center gap-1.5">
      <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
        {show ? value : "••••••••"}
      </span>
      <button
        onClick={() => setShow(!show)}
        className="shrink-0 rounded px-1 font-mono text-[11px] text-zinc-500 hover:bg-accent hover:text-zinc-200"
        title={show ? "ocultar" : "mostrar"}
      >
        {show ? "ocultar" : "mostrar"}
      </button>
      <CopyButton text={value} />
    </span>
  );
}

// EmptyState con acción (todas las listas vacías lo usan).
export function EmptyState({ title, hint, action }: { title: string; hint?: string; action?: React.ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-2 p-10 text-center">
      <p className="text-sm font-medium">{title}</p>
      {hint && <p className="max-w-sm text-xs text-muted-foreground">{hint}</p>}
      {action}
    </div>
  );
}
