import { ExternalLink } from "lucide-react";
import { PageHeader, Section } from "../../shared/components/patterns";
import { Kbd } from "../../shared/components/ui";
import { useI18n } from "../../shared/i18n";

// Ayuda in-app: atajos, glosario mínimo y enlaces al manual. Cero ayuda
// contextual antes; esta página es el hogar único (los docs largos siguen
// en docs/ del repo).
const SHORTCUTS: { keys: string[]; labelKey: string }[] = [
  { keys: ["⌘", "K"], labelKey: "help.shortcuts.search" },
  { keys: ["/"], labelKey: "help.shortcuts.slash" },
  { keys: ["N"], labelKey: "help.shortcuts.new" },
  { keys: ["Esc"], labelKey: "help.shortcuts.close" },
];

const CONCEPTS: { titleKey: string; descKey: string }[] = [
  { titleKey: "help.concepts.managed", descKey: "help.concepts.managedDesc" },
  { titleKey: "help.concepts.external", descKey: "help.concepts.externalDesc" },
  { titleKey: "help.concepts.deploy", descKey: "help.concepts.deployDesc" },
  { titleKey: "help.concepts.rollback", descKey: "help.concepts.rollbackDesc" },
  { titleKey: "help.concepts.route", descKey: "help.concepts.routeDesc" },
  { titleKey: "help.concepts.port", descKey: "help.concepts.portDesc" },
  { titleKey: "help.concepts.waf", descKey: "help.concepts.wafDesc" },
  { titleKey: "help.concepts.mcp", descKey: "help.concepts.mcpDesc" },
  { titleKey: "help.concepts.cron", descKey: "help.concepts.cronDesc" },
];

const DOCS: { labelKey: string; href: string }[] = [
  { labelKey: "help.docs.readme", href: "https://github.com/carlos-burelo/rkt" },
  { labelKey: "help.docs.cli", href: "https://github.com/carlos-burelo/rkt/blob/main/docs/CLI.md" },
  { labelKey: "help.docs.arch", href: "https://github.com/carlos-burelo/rkt/blob/main/docs/ARCHITECTURE.md" },
];

export function Help() {
  const { t } = useI18n();
  return (
    <div className="space-y-4">
      <PageHeader title={t("help.title")} description={t("help.description")} />

      <Section title={t("help.shortcuts.title")} description={t("help.shortcuts.desc")}>
        <ul className="divide-y divide-border/60">
          {SHORTCUTS.map((s) => (
            <li key={s.labelKey} className="flex items-center justify-between gap-3 py-1.5">
              <span className="font-mono text-[11px]">{t(s.labelKey)}</span>
              <span className="flex items-center gap-0.5">
                {s.keys.map((k) => (
                  <Kbd key={k}>{k}</Kbd>
                ))}
              </span>
            </li>
          ))}
        </ul>
      </Section>

      <Section title={t("help.concepts.title")} description={t("help.concepts.desc")}>
        <dl className="grid grid-cols-1 gap-px border border-border bg-border md:grid-cols-2">
          {CONCEPTS.map((c) => (
            <div key={c.titleKey} className="bg-card/45 px-3 py-2.5">
              <dt className="text-[12px] font-medium">{t(c.titleKey)}</dt>
              <dd className="mt-1 text-[12px] leading-relaxed text-muted-foreground">{t(c.descKey)}</dd>
            </div>
          ))}
        </dl>
      </Section>

      <Section title={t("help.docs.title")} description={t("help.docs.desc")}>
        <ul className="space-y-1.5">
          {DOCS.map((d) => (
            <li key={d.labelKey}>
              <a
                href={d.href}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1.5 font-mono text-[11px] text-foreground/80 hover:text-signal hover:underline"
              >
                {t(d.labelKey)}
                <ExternalLink className="h-3 w-3" aria-hidden />
              </a>
            </li>
          ))}
        </ul>
        <p className="mt-3 font-mono text-[10px] leading-relaxed text-muted-foreground">{t("help.docs.note")}</p>
      </Section>
    </div>
  );
}
