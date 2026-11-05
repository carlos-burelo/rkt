import { useState } from "react";
import { CopyButton } from "./feedback";
import { rel } from "../lib/time";
import { cn } from "@/lib/utils";
import { useI18n } from "../i18n";

// PageHeader: folio de terminal. La regla de señal es la firma, no un kicker.
export function PageHeader({
  title,
  description,
  actions,
  meta,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
  meta?: React.ReactNode;
}) {
  return (
    <header className="flex flex-col gap-3 border-b border-border pb-4 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <div className="flex items-center gap-2.5">
          <span aria-hidden className="h-1 w-4 shrink-0 bg-signal" />
          <h1 className="text-[24px] leading-none font-semibold tracking-[-0.03em]">{title}</h1>
        </div>
        {description && <p className="mt-2 max-w-[78ch] text-[13px] leading-relaxed text-muted-foreground">{description}</p>}
      </div>
      <div className="flex flex-wrap items-center gap-2 sm:justify-end">
        {meta}
        {actions}
      </div>
    </header>
  );
}

// Section: bloque de contenido con cabecera hairline, sin sombra.
export function Section({
  title,
  description,
  actions,
  children,
  accent = false,
  className,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
  accent?: boolean;
  className?: string;
}) {
  return (
    <section className={cn("border border-border bg-card/45", accent && "border-t-signal/70", className)}>
      <header className="flex flex-wrap items-center gap-3 border-b border-border bg-secondary/20 px-3 py-2">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <span aria-hidden className={cn("h-1.5 w-1.5 shrink-0", accent ? "bg-signal" : "bg-muted-foreground/40")} />
          <h2 className="text-[12px] font-medium tracking-tight">{title}</h2>
          {description && <p className="hidden truncate text-[11px] text-muted-foreground sm:block">{description}</p>}
        </div>
        {actions}
      </header>
      <div className="p-3">{children}</div>
    </section>
  );
}

const TONE_TEXT = {
  signal: "text-signal",
  ok: "text-ok",
  warn: "text-warn",
  bad: "text-bad",
} as const;

const TONE_MARK = {
  signal: "bg-signal",
  ok: "bg-ok",
  warn: "bg-warn",
  bad: "bg-bad",
} as const;

// Stat: celda de libro, no tarjeta. Número tabular, etiqueta mono, ventana honesta.
export function Stat({
  value,
  label,
  sub,
  tone,
  className,
}: {
  value: string;
  label: string;
  sub?: string;
  tone?: keyof typeof TONE_TEXT;
  className?: string;
}) {
  return (
    <div className={cn("min-w-0 bg-card/55 px-4 py-3", className)}>
      <div className="flex items-center gap-2">
        {tone && <span aria-hidden className={cn("h-1 w-1 shrink-0", TONE_MARK[tone])} />}
        <span className="micro truncate">{label}</span>
      </div>
      <p className={cn("mt-2 font-mono text-[26px] leading-none font-medium tracking-[-0.04em] tnum", tone && TONE_TEXT[tone])}>
        {value}
      </p>
      {sub && <p className="mt-1.5 truncate font-mono text-[10px] text-muted-foreground">{sub}</p>}
    </div>
  );
}

export function StatGrid({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div className={cn("grid grid-cols-2 gap-px border border-border bg-border lg:grid-cols-4", className)}>
      {children}
    </div>
  );
}

export function Segmented<T extends string>({
  value,
  onChange,
  items,
  label,
}: {
  value: T;
  onChange: (v: T) => void;
  items: { value: T; label: string; count?: number }[];
  label: string;
}) {
  return (
    <div className="inline-flex items-center border border-border" role="group" aria-label={label}>
      {items.map((it) => {
        const active = it.value === value;
        return (
          <button
            key={it.value}
            type="button"
            onClick={() => onChange(it.value)}
            aria-pressed={active}
            className={cn(
              "inline-flex h-7 items-center gap-1.5 border-r border-border px-2.5 font-mono text-[10px] tracking-[0.08em] uppercase transition-colors last:border-r-0",
              active
                ? "bg-secondary font-medium text-foreground shadow-[inset_0_-1px_0_0_var(--signal)]"
                : "text-muted-foreground hover:bg-accent hover:text-foreground",
            )}
          >
            {it.label}
            {it.count !== undefined && <span className="tnum opacity-70">{it.count}</span>}
          </button>
        );
      })}
    </div>
  );
}

export function Meter({
  label,
  value,
  frac,
  tone = "ok",
}: {
  label: string;
  value: string;
  frac?: number;
  tone?: "ok" | "warn" | "bad" | "signal";
}) {
  const bar = tone === "bad" ? "bg-bad" : tone === "warn" ? "bg-warn" : tone === "signal" ? "bg-signal" : "bg-ok";
  const pct = frac === undefined ? 0 : Math.min(100, Math.max(1, frac * 100));
  return (
    <div>
      <div className="flex items-baseline justify-between gap-3">
        <span className="micro">{label}</span>
        <span className="tnum font-mono text-[11px] text-foreground">{value}</span>
      </div>
      {frac === undefined ? (
        <div aria-hidden className="mt-2 h-px bg-border" />
      ) : (
        <div
          className="mt-2 h-1.5 overflow-hidden bg-secondary/80"
          role="progressbar"
          aria-label={label}
          aria-valuenow={Math.round(pct)}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuetext={value}
        >
          <div className={cn("h-full transition-[width] duration-300", bar)} style={{ width: `${pct}%` }} />
        </div>
      )}
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
  const { t } = useI18n();
  const [show, setShow] = useState(false);
  const toggle = show ? t("shared.envSecret.hide") : t("shared.envSecret.show");
  return (
    <span className="flex min-w-0 flex-1 items-center gap-1.5">
      <span className="tnum min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground">
        {show ? value : "••••••••"}
      </span>
      <button
        onClick={() => setShow(!show)}
        className="shrink-0 rounded-[1px] px-1 font-mono text-[10px] text-muted-foreground hover:bg-accent hover:text-foreground"
        title={toggle}
      >
        {toggle}
      </button>
      <CopyButton text={value} />
    </span>
  );
}

// EmptyState terminal: folio sin registros, no tarjeta dashed.
export function EmptyState({ title, hint, action }: { title: string; hint?: string; action?: React.ReactNode }) {
  const { t } = useI18n();
  return (
    <div className="flex flex-col items-center gap-2 border border-border bg-card/30 px-6 py-10 text-center">
      <span className="micro text-muted-foreground/55">{t("shared.emptyState.none")}</span>
      <p className="text-[13px] font-medium">{title}</p>
      {hint && <p className="tnum max-w-[64ch] font-mono text-[11px] leading-relaxed text-muted-foreground">{hint}</p>}
      {action}
    </div>
  );
}
