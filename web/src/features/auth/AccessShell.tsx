import { BrandMark } from "../../shared/components/ui";

// Marco de acceso: la terminal se presenta como manifiesto, no como landing.
export function AccessShell({
  kicker,
  title,
  description,
  children,
  footer,
}: {
  kicker: string;
  title: string;
  description: string;
  children: React.ReactNode;
  footer: React.ReactNode;
}) {
  return (
    <div className="flex min-h-screen flex-col lg:flex-row">
      <aside className="relative hidden overflow-hidden border-r border-border bg-ink/70 lg:flex lg:w-[44%] lg:flex-col lg:justify-between lg:p-10">
        <div aria-hidden className="grid-paper pointer-events-none absolute inset-0 opacity-60" />
        <div aria-hidden className="pointer-events-none absolute inset-x-0 top-0 h-px bg-signal/60" />
        <div className="relative">
          <div className="flex items-center gap-3">
            <span className="flex h-9 w-9 items-center justify-center border border-signal/40 bg-signal/10 text-signal">
              <BrandMark className="h-5 w-5" />
            </span>
            <div>
              <p className="text-[15px] leading-none font-semibold tracking-tight">rkt</p>
              <p className="micro mt-1">panel de control</p>
            </div>
          </div>
          <div className="mt-16 max-w-sm">
            <p className="folio">tus apps, en un solo sitio</p>
            <h2 className="mt-3 text-[32px] leading-[1.05] font-semibold tracking-[-0.04em] text-balance">
              Cada app con su puerto, su dominio y un supervisor que la mantiene en marcha.
            </h2>
            <p className="mt-4 max-w-[36ch] text-[12px] leading-relaxed text-muted-foreground">
              Despliega, revierte y publica sin editar YAML. Solo mostramos lo que medimos de verdad.
            </p>
          </div>
          <div className="mt-10 max-w-sm border border-border/80">
            {[
              ["01", "operar", "despliegues · registros · CI"],
              ["02", "servir", "dominios · proxy · entorno"],
              ["03", "observar", "tráfico · alertas"],
            ].map(([n, k, d]) => (
              <div key={k} className="flex items-baseline gap-3 border-b border-border/60 px-3 py-2 last:border-0">
                <span className="folio">{n}</span>
                <span className="font-mono text-[10px] tracking-[0.08em] uppercase">{k}</span>
                <span className="flex-1" />
                <span className="font-mono text-[9px] text-muted-foreground">{d}</span>
              </div>
            ))}
          </div>
        </div>
        <div className="relative flex items-end justify-between gap-4 border-t border-border/70 pt-4">
          <p className="font-mono text-[9px] leading-relaxed text-muted-foreground/70">
            acceso restringido · sesión con cookie segura
            <br />
            sin usuarios, rkt no opera
          </p>
          <span className="tnum font-mono text-[9px] text-muted-foreground/50">rkt / v1</span>
        </div>
      </aside>

      <main className="flex flex-1 items-center justify-center px-5 py-10">
        <div className="w-full max-w-[22rem]">
          <div className="mb-8 flex items-center gap-2.5 lg:hidden">
            <span className="flex h-8 w-8 items-center justify-center border border-signal/40 bg-signal/10 text-signal">
              <BrandMark className="h-4 w-4" />
            </span>
            <span className="text-[13px] font-semibold tracking-tight">rkt</span>
            <span className="micro">panel</span>
          </div>
          <header className="border-b border-border pb-4">
            <div className="flex items-center gap-2.5">
              <span aria-hidden className="h-1 w-4 bg-signal" />
              <span className="micro">{kicker}</span>
            </div>
            <h1 className="mt-2 text-[24px] leading-none font-semibold tracking-[-0.03em]">{title}</h1>
            <p className="mt-2 text-[12px] leading-relaxed text-muted-foreground">{description}</p>
          </header>
          <div className="mt-5">{children}</div>
          <div className="mt-8 border-t border-border pt-3">{footer}</div>
        </div>
      </main>
    </div>
  );
}
