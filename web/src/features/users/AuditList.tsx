import { Section } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import { useUsers } from "./users.store";

// Auditoría reciente: lectura pura del store.
export function AuditList() {
  const { t } = useI18n();
  const audit = useUsers((s) => s.audit);
  return (
    <Section title={t("system.users.auditTitle")} description={t("system.users.auditDesc")}>
      <div className="border border-border">
        {audit.slice(0, 20).map((a) => (
          <div key={a.id} className="grid grid-cols-[8rem_minmax(0,1fr)_auto] items-baseline gap-2 border-b border-border/60 px-3 py-1.5 last:border-0">
            <span className="tnum font-mono text-[9px] text-muted-foreground">{a.ts.replace("T", " ").slice(0, 19)}</span>
            <span className="min-w-0 truncate font-mono text-[10px]">
              <b className="text-foreground/80">{a.actor}</b> · {a.action} · {a.target}
            </span>
            <span className="font-mono text-[9px] text-muted-foreground">{a.ip}</span>
          </div>
        ))}
        {audit.length === 0 && <p className="px-3 py-3 font-mono text-[10px] text-muted-foreground">{t("system.users.noAudit")}</p>}
      </div>
    </Section>
  );
}
