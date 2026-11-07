import { Button } from "@/components/ui/button";
import { Section } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import { useSettings } from "./settings.store";

// GitHub App: estado y accesos. Solo lectura del store.
export function GithubCard() {
  const { t } = useI18n();
  const gh = useSettings((s) => s.gh);
  return (
    <Section
      title={t("system.settings.githubSection")}
      accent={!!gh?.configured && gh.repository_selection !== "all"}
      description={t("system.settings.githubDesc")}
      actions={
        gh?.configured && gh.install_url ? (
          <a
            href={gh.install_url}
            target="_blank"
            rel="noreferrer"
            className="font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:text-foreground"
          >
            {t("system.settings.manage")}
          </a>
        ) : undefined
      }
    >
      {!gh && <p className="font-mono text-[10px] text-muted-foreground">{t("system.settings.githubChecking")}</p>}
      {gh && !gh.configured && (
        <div className="space-y-2">
          {gh.stale && (
            <p className="border border-warn/35 bg-warn/5 px-3 py-2 text-[12px] text-warn">
              {t("system.settings.githubStale")}
            </p>
          )}
          <p className="text-[12px] text-muted-foreground">
            {t("system.settings.noAppBefore")}{" "}
            <b className="text-foreground">All repositories</b>{t("system.settings.noAppAfter")}
          </p>
          {gh.setup_url ? (
            <div className="space-y-2">
              <a href={gh.setup_url} target="_blank" rel="noreferrer">
                <Button size="sm" variant="outline">
                  {t("system.settings.createApp")}
                </Button>
              </a>
              {gh.local && (
                <p className="font-mono text-[10px] text-muted-foreground">
                  {t("system.settings.localMode")}
                </p>
              )}
            </div>
          ) : (
            <p className="border border-warn/30 bg-warn/5 px-2 py-1.5 font-mono text-[10px] text-warn">
              {t("system.settings.baseDomainBefore")} <b>base_domain</b> {t("system.settings.baseDomainAfter")}
            </p>
          )}
        </div>
      )}
      {gh && gh.configured && (
        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-px border border-border bg-border lg:grid-cols-4">
            <div className="bg-card/45 px-3 py-2">
              <p className="micro">{t("system.settings.account")}</p>
              <p className="mt-1 truncate font-mono text-[11px]">{gh.account || "—"}</p>
            </div>
            <div className="bg-card/45 px-3 py-2">
              <p className="micro">{t("system.settings.app")}</p>
              <p className="mt-1 truncate font-mono text-[11px]">{gh.app_slug || gh.app_id}</p>
            </div>
            <div className="bg-card/45 px-3 py-2">
              <p className="micro">{t("system.settings.repositories")}</p>
              <p className="mt-1 font-mono text-[11px]">{gh.repositories ?? "—"}</p>
            </div>
            <div className="bg-card/45 px-3 py-2">
              <p className="micro">{t("system.settings.scope")}</p>
              <p className={`mt-1 font-mono text-[11px] ${gh.repository_selection === "all" ? "text-ok" : gh.repository_selection ? "text-warn" : "text-muted-foreground"}`}>
                {gh.repository_selection === "all" ? t("common.all") : gh.repository_selection === "selected" ? t("system.settings.scopeSelected") : t("system.settings.scopeMissing")}
              </p>
            </div>
          </div>
          {!gh.repository_selection ? (
            <div className="border border-border bg-card/30 px-3 py-2 text-[12px]">
              <p className="text-muted-foreground">
                {t("system.settings.installBefore")} <b className="text-foreground">All repositories</b>{t("system.settings.installAfter")}
              </p>
              {gh.install_url && (
                <a href={gh.install_url} target="_blank" rel="noreferrer" className="mt-2 inline-block">
                  <Button size="sm" variant="outline">
                    {t("system.settings.installAll")}
                  </Button>
                </a>
              )}
            </div>
          ) : gh.repository_selection !== "all" ? (
            <div className="border border-warn/35 bg-warn/5 px-3 py-2 text-[12px]">
              <p className="font-mono text-[10px] tracking-[0.08em] text-warn uppercase">{t("system.settings.partial")}</p>
              <p className="mt-1 text-muted-foreground">
                {t("system.settings.partialBefore")}{" "}
                <b className="text-foreground">All repositories</b> {t("system.settings.partialAfter")}
              </p>
              {gh.install_url && (
                <a href={gh.install_url} target="_blank" rel="noreferrer" className="mt-2 inline-block">
                  <Button size="sm" variant="outline">
                    {t("system.settings.changeAccess")}
                  </Button>
                </a>
              )}
            </div>
          ) : (
            <p className="font-mono text-[10px] text-ok">{t("system.settings.fullAccess")}</p>
          )}
          {gh.error && <p className="font-mono text-[10px] text-warn">{t("system.settings.githubError", { error: gh.error })}</p>}
        </div>
      )}
    </Section>
  );
}
