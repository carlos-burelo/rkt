import { useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import { Dot, MicroLink, stateOf } from "../../shared/components/ui";
import { useI18n } from "../../shared/i18n";
import type { AppStatus } from "../../shared/lib/types";

// Banners de atención (apps caídas/degradadas) y certs por vencer.
export function AttentionBanner({ apps }: { apps: AppStatus[] }) {
  const { t, tn } = useI18n();
  const navigate = useNavigate();
  const openApp = (name: string) => navigate(`/app/${name}`);
  const failedApps = apps.filter((a) => a.failed);
  const degradedApps = apps.filter((a) => !a.failed && stateOf(a) === "degraded");
  if (failedApps.length === 0 && degradedApps.length === 0) return null;
  return (
    <section
      aria-label={t("overview.attention.aria")}
      className={failedApps.length > 0 ? "border border-bad/30 bg-bad/[0.055] px-3 py-2" : "border border-warn/30 bg-warn/[0.05] px-3 py-2"}
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <p className="flex min-w-0 flex-1 basis-full items-center gap-2 font-mono text-[11px] leading-relaxed sm:basis-0">
          <Dot status={failedApps.length > 0 ? "failed" : "degraded"} />
          <span className="min-w-0">
            {failedApps.length > 0 && (
              <span className="text-bad">{tn("overview.stuckFem", failedApps.length)}</span>
            )}
            {failedApps.length > 0 && degradedApps.length > 0 && (
              <span className="text-muted-foreground"> · </span>
            )}
            {degradedApps.length > 0 && (
              <span className="text-warn">{tn("overview.degraded", degradedApps.length)}</span>
            )}
            <span className="text-muted-foreground"> · {[...failedApps, ...degradedApps].map((a) => a.name).join(", ")}</span>
          </span>
        </p>
        <div className="flex shrink-0 items-center gap-3">
          <Button size="xs" variant="outline" onClick={() => openApp([...failedApps, ...degradedApps][0].name)}>
            {t("overview.attention.open")}
          </Button>
          <MicroLink to="/alerts" className="text-[10px]">{t("overview.attention.alerts")}</MicroLink>
        </div>
      </div>
    </section>
  );
}

export function TlsBanner({ items }: { items: { host: string; date: string }[] }) {
  const { t, tn } = useI18n();
  if (items.length === 0) return null;
  return (
    <section className="flex flex-wrap items-center gap-3 border border-warn/30 bg-warn/[0.05] px-3 py-2">
      <p className="min-w-0 flex-1 font-mono text-[11px] text-warn">
        {tn("overview.tlsSoon", items.length)} · {items.map((x) => `${x.host} (${x.date})`).join(", ")}
      </p>
      <MicroLink to="/routes" className="text-[10px]">{t("overview.tlsSoonAction")}</MicroLink>
    </section>
  );
}
