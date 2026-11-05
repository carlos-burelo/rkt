import { BrandMark } from "../../shared/components/ui";
import { useI18n } from "../../shared/i18n";
import { ThemeToggle } from "../../shared/theme/ThemeToggle";

// Marco de acceso: formulario operativo, sin landing promocional.
export function AccessShell({
  title,
  description,
  children,
  footer,
}: {
  title: string;
  description: string;
  children: React.ReactNode;
  footer: React.ReactNode;
}) {
  const { t } = useI18n();
  return (
    <div className="flex min-h-screen flex-col lg:flex-row">
      <aside className="relative hidden overflow-hidden border-r border-border bg-card/50 lg:flex lg:w-[44%] lg:flex-col lg:justify-between lg:p-10">
        <div aria-hidden className="pointer-events-none absolute inset-x-0 top-0 h-px bg-signal/60" />
        <div className="relative">
          <div className="flex items-center gap-3">
            <span className="flex h-9 w-9 items-center justify-center border border-signal/40 bg-signal/10 text-signal">
              <BrandMark className="h-5 w-5" />
            </span>
            <div>
              <p className="text-[15px] leading-none font-semibold tracking-tight">rkt</p>
              <p className="micro mt-1">{t("system.access.panelControl")}</p>
            </div>
          </div>
        </div>
        <div className="relative flex items-end justify-between gap-4 border-t border-border/70 pt-4">
          <p className="font-mono text-[9px] leading-relaxed text-muted-foreground/70">
            {t("system.access.restricted")}
            <br />
            {t("system.access.noUsers")}
          </p>
          <span className="tnum font-mono text-[9px] text-muted-foreground/50">rkt / v1</span>
        </div>
      </aside>

      <main className="relative flex flex-1 items-center justify-center px-5 py-10">
        <ThemeToggle compact className="absolute top-4 right-4" />
        <div className="w-full max-w-[22rem]">
          <div className="mb-8 flex items-center gap-2.5 lg:hidden">
            <span className="flex h-8 w-8 items-center justify-center border border-signal/40 bg-signal/10 text-signal">
              <BrandMark className="h-4 w-4" />
            </span>
            <span className="text-[13px] font-semibold tracking-tight">rkt</span>
            <span className="micro">{t("system.access.panel")}</span>
          </div>
          <header className="border-b border-border pb-4">
            <div className="flex items-center gap-2.5">
              <span aria-hidden className="h-1 w-4 bg-signal" />
              <h1 className="text-[24px] leading-none font-semibold tracking-[-0.03em]">{title}</h1>
            </div>
            <p className="mt-2 text-[12px] leading-relaxed text-muted-foreground">{description}</p>
          </header>
          <div className="mt-5">{children}</div>
          <div className="mt-8 border-t border-border pt-3">{footer}</div>
        </div>
      </main>
    </div>
  );
}
