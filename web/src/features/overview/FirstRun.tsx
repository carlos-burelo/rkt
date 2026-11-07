import { useNavigate } from "react-router";
import { ArrowRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useI18n } from "../../shared/i18n";
import type { AppMeta, AppStatus, Deployment } from "../../shared/lib/types";

// Primer arranque: guía de 3 pasos. Puro presentacional.
export function FirstRun({ apps, metas, deps }: { apps: AppStatus[]; metas: AppMeta[]; deps: Deployment[] }) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const hasApp = apps.length > 0;
  const hasDomain = metas.some((m) => (m.domains ?? []).length > 0);
  const hasDeploy = deps.some((d) => d.status === "ok");
  const done = [hasApp, hasDomain, hasDeploy].filter(Boolean).length;
  if (done === 3) return null;
  const steps = [
    { ok: hasApp, label: t("overview.firstRun.stepApp"), hint: t("overview.firstRun.stepAppHint") },
    { ok: hasDomain, label: t("overview.firstRun.stepDomain"), hint: t("overview.firstRun.stepDomainHint") },
    { ok: hasDeploy, label: t("overview.firstRun.stepDeploy"), hint: t("overview.firstRun.stepDeployHint") },
  ];
  return (
    <section className="border border-border bg-card/45">
      <header className="flex items-center gap-3 border-b border-border bg-secondary/20 px-3 py-2">
        <span aria-hidden className="h-1.5 w-1.5 bg-signal" />
        <h2 className="text-[12px] font-medium">{t("overview.firstRun.title")}</h2>
        <span className="flex-1" />
        <span className="tnum font-mono text-[10px] text-muted-foreground">
          {t("overview.firstRun.progress", { done })}
        </span>
      </header>
      <div className="grid gap-px bg-border md:grid-cols-3">
        {steps.map((s, i) => (
          <div key={s.label} className="bg-card/55 p-4">
            <div className="flex items-center justify-between gap-2">
              <span className="folio">0{i + 1}</span>
              <span className={s.ok ? "font-mono text-[10px] text-ok" : "font-mono text-[10px] text-muted-foreground"}>
                {s.ok ? t("overview.firstRun.ready") : t("overview.firstRun.pending")}
              </span>
            </div>
            <p className="mt-3 text-[13px] font-medium">{s.label}</p>
            <p className="mt-1 font-mono text-[10px] text-muted-foreground">{s.hint}</p>
          </div>
        ))}
      </div>
      {!hasApp && (
        <div className="border-t border-border px-3 py-2.5">
          <Button size="sm" onClick={() => navigate("/apps/new")}>
            {t("overview.firstRun.create")} <ArrowRight className="h-3 w-3" />
          </Button>
        </div>
      )}
    </section>
  );
}
