import { useState } from "react";
import { CopyButton } from "./feedback";
import { rel } from "../lib/time";
import { Card, CardContent, CardHeader } from "@/components/ui/card";

// PageHeader: título + descripción + acciones. Sin kicker.
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
        <h2 className="text-balance text-xl font-semibold tracking-tight">{title}</h2>
        {description && <p className="mt-1 max-w-[75ch] text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

// Section: tarjeta hairline con encabezado. Sin sombra dura.
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
    <Card className="gap-0 py-0">
      <CardHeader className="flex-row items-start gap-3 space-y-0 border-b px-5 py-4">
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium">{title}</h3>
          {description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
        </div>
        {actions}
      </CardHeader>
      <CardContent className="px-5 py-4">{children}</CardContent>
    </Card>
  );
}

// Stat: placa de escala fija para métricas. value tabular, label abajo,
// sub opcional (delta honesto con onset). Todas las placas del mismo
// grupo miden lo mismo.
export function Stat({
  value,
  label,
  sub,
  tone,
}: {
  value: string;
  label: string;
  sub?: string;
  tone?: "signal" | "ok" | "warn" | "bad";
}) {
  const toneCls =
    tone === "bad"
      ? "text-red-300"
      : tone === "warn"
        ? "text-amber-300"
        : tone === "ok"
          ? "text-emerald-300"
          : tone === "signal"
            ? "text-signal"
            : "";
  return (
    <div className="rounded-lg border bg-card px-4 pb-3 pt-3.5">
      <p className={`tnum text-2xl font-semibold tracking-tight ${toneCls}`}>{value}</p>
      <p className="mt-1 text-xs text-muted-foreground">{label}</p>
      {sub && <p className="tnum mt-0.5 font-mono text-[11px] text-muted-foreground">{sub}</p>}
    </div>
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
      <span className="tnum min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
        {show ? value : "••••••••"}
      </span>
      <button
        onClick={() => setShow(!show)}
        className="shrink-0 rounded px-1 font-mono text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground"
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
      {hint && <p className="max-w-[60ch] text-xs text-muted-foreground">{hint}</p>}
      {action}
    </div>
  );
}
