import { useState, useEffect, useRef } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Section } from "../../shared/components/patterns";
import { MicroLink } from "../../shared/components/ui";
import { useI18n } from "../../shared/i18n";
import { useSettings } from "./settings.store";
import { disconnectGithub, testGithub, type GithubTestResult } from "./api";
import { useUI } from "../../app/ui";

// GitHub App: estado, validaciones, prueba en vivo y desvinculación segura.
export function GithubCard() {
  const { t } = useI18n();
  const gh = useSettings((s) => s.gh);
  const [waitingLink, setWaitingLink] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<GithubTestResult | null>(null);
  const [confirmDisconnect, setConfirmDisconnect] = useState(false);
  const [disconnecting, setDisconnecting] = useState(false);
  const timerRef = useRef<number | null>(null);

  // Polling cuando se abre la ventana de GitHub
  useEffect(() => {
    if (!waitingLink) {
      if (timerRef.current) clearInterval(timerRef.current);
      return;
    }
    timerRef.current = window.setInterval(async () => {
      await useSettings.getState().load();
      const current = useSettings.getState().gh;
      if (current?.configured) {
        setWaitingLink(false);
        toast.success(t("system.settings.githubConnected"));
      }
    }, 2500);

    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [waitingLink, t]);

  // Si ya quedó configurado, apagar la espera
  useEffect(() => {
    if (gh?.configured && waitingLink) {
      setWaitingLink(false);
    }
  }, [gh?.configured, waitingLink]);

  const onStartConnect = () => {
    if (gh?.setup_url) {
      window.open(gh.setup_url, "_blank", "noopener");
      setWaitingLink(true);
      setTestResult(null);
    }
  };

  const onTest = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await testGithub();
      setTestResult(res);
      if (res.ok) {
        toast.success(t("system.settings.githubTestOk", { ms: res.latency_ms ?? 0, repos: res.repositories ?? 0 }));
      }
    } catch (e: any) {
      setTestResult({ ok: false, error: e?.message || String(e) });
    } finally {
      setTesting(false);
    }
  };

  const onDisconnect = async () => {
    setDisconnecting(true);
    try {
      await disconnectGithub();
      setConfirmDisconnect(false);
      setTestResult(null);
      await useSettings.getState().load();
      toast.success(t("system.settings.githubDisconnected"));
    } catch (e: any) {
      useUI.getState().fail(e);
    } finally {
      setDisconnecting(false);
    }
  };

  return (
    <Section
      title={t("system.settings.githubSection")}
      accent={!!gh?.configured && (gh.repository_selection !== "all" || gh.install_pending)}
      description={t("system.settings.githubDesc")}
      actions={
        gh?.configured && gh.install_url ? (
          <div className="flex items-center gap-3">
            <MicroLink href={gh.install_url}>{t("system.settings.manage")}</MicroLink>
          </div>
        ) : undefined
      }
    >
      {!gh && <p className="font-mono text-[10px] text-muted-foreground">{t("system.settings.githubChecking")}</p>}

      {gh && !gh.configured && (
        <div className="space-y-3">
          {gh.stale && (
            <p className="border border-warn/35 bg-warn/5 px-3 py-2 text-[12px] text-warn">
              {t("system.settings.githubStale")}
            </p>
          )}

          <p className="text-[12px] text-muted-foreground">
            {t("system.settings.noAppBefore")}{" "}
            <b className="text-foreground">All repositories</b>{t("system.settings.noAppAfter")}
          </p>

          {gh.warning && (
            <p className="border border-warn/30 bg-warn/5 px-2.5 py-1.5 font-mono text-[11px] text-warn">
              {gh.warning}
            </p>
          )}

          {gh.setup_url ? (
            <div className="space-y-2">
              <div className="flex items-center gap-3">
                <Button size="sm" variant="outline" onClick={onStartConnect} disabled={waitingLink}>
                  {waitingLink ? t("system.settings.githubWaiting") : t("system.settings.createApp")}
                </Button>
                {waitingLink && (
                  <button
                    type="button"
                    onClick={() => setWaitingLink(false)}
                    className="font-mono text-[10px] text-muted-foreground hover:underline"
                  >
                    {t("system.settings.githubCancelWait")}
                  </button>
                )}
              </div>
              {waitingLink && (
                <p className="font-mono text-[11px] text-signal animate-pulse">
                  {t("system.settings.githubWaitingHint")}
                </p>
              )}
              {gh.local && !waitingLink && (
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
          {gh.warning && (
            <p className="border border-warn/30 bg-warn/5 px-2.5 py-1.5 font-mono text-[11px] text-warn">
              {gh.warning}
            </p>
          )}

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

          <div className="flex items-center justify-between border border-border bg-card/20 px-3 py-2 text-[11px]">
            <span className="font-mono text-muted-foreground">{t("system.settings.githubWebhook")}:</span>
            <span className={`font-mono ${gh.webhook_active ? "text-ok" : "text-muted-foreground"}`}>
              {gh.webhook_active ? t("system.settings.githubWebhookActive") : t("system.settings.githubWebhookLocal")}
            </span>
          </div>

          {gh.install_pending || !gh.repository_selection ? (
            <div className="border border-warn/35 bg-warn/5 px-3 py-2.5 text-[12px] space-y-2">
              <p className="text-warn font-medium">
                {t("system.settings.githubPendingInstall")}
              </p>
              <p className="text-muted-foreground">
                {t("system.settings.installBefore")} <b className="text-foreground">All repositories</b>{t("system.settings.installAfter")}
              </p>
              {gh.install_url && (
                <a href={gh.install_url} target="_blank" rel="noreferrer" className="inline-block">
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

          {gh.error && (
            <p className="border border-warn/40 bg-warn/5 px-2.5 py-1.5 font-mono text-[11px] text-warn">
              {t("system.settings.githubError", { error: gh.error })}
            </p>
          )}

          {testResult && (
            <div className={`border px-3 py-2 font-mono text-[11px] ${testResult.ok ? "border-ok/40 bg-ok/5 text-ok" : "border-warn/40 bg-warn/5 text-warn"}`}>
              {testResult.ok
                ? t("system.settings.githubTestOk", { ms: testResult.latency_ms ?? 0, repos: testResult.repositories ?? 0 })
                : t("system.settings.githubTestErr", { error: testResult.error ?? "error desconocido" })}
            </div>
          )}

          <div className="flex flex-wrap items-center justify-between gap-2 pt-1">
            <Button size="sm" variant="outline" onClick={onTest} disabled={testing}>
              {testing ? t("system.settings.githubTesting") : t("system.settings.githubTest")}
            </Button>

            {!confirmDisconnect ? (
              <Button size="sm" variant="ghost" className="text-muted-foreground hover:text-bad" onClick={() => setConfirmDisconnect(true)}>
                {t("system.settings.githubDisconnect")}
              </Button>
            ) : (
              <div className="flex items-center gap-2 border border-bad/40 bg-bad/5 px-2.5 py-1">
                <span className="font-mono text-[10px] text-bad">{t("system.settings.githubDisconnectConfirm")}</span>
                <Button size="xs" variant="destructive" onClick={onDisconnect} disabled={disconnecting}>
                  {disconnecting ? t("system.settings.githubDisconnecting") : t("common.delete")}
                </Button>
                <Button size="xs" variant="ghost" onClick={() => setConfirmDisconnect(false)} disabled={disconnecting}>
                  {t("common.cancel")}
                </Button>
              </div>
            )}
          </div>
        </div>
      )}
    </Section>
  );
}
