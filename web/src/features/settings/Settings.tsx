import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";
import { getSettings, setSetting, getGithubStatus, GithubStatus } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader, Section } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { PathInput } from "../../shared/components/PathPicker";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import { McpCard } from "./McpCard";

const DESCRIPTION_KEYS: Record<string, string> = {
  node_bin: "system.settings.desc.node_bin",
  python_bin: "system.settings.desc.python_bin",
  base_domain: "system.settings.desc.base_domain",
  port_pool: "system.settings.desc.port_pool",
  site_name: "system.settings.desc.site_name",
  quota_reqs: "system.settings.desc.quota_reqs",
  quota_bytes: "system.settings.desc.quota_bytes",
  public_ip: "system.settings.desc.public_ip",
};

const LABELS: Record<string, string> = {
  node_bin: "system.settings.label.node_bin",
  python_bin: "system.settings.label.python_bin",
  base_domain: "system.settings.label.base_domain",
  port_pool: "system.settings.label.port_pool",
  site_name: "system.settings.label.site_name",
  quota_reqs: "system.settings.label.quota_reqs",
  quota_bytes: "system.settings.label.quota_bytes",
  public_ip: "system.settings.label.public_ip",
};

const GROUPS: { title: string; desc: string; keys: string[] }[] = [
  { title: "system.settings.groupNetwork", desc: "system.settings.groupNetworkDesc", keys: ["base_domain", "public_ip", "site_name"] },
  { title: "system.settings.groupProcess", desc: "system.settings.groupProcessDesc", keys: ["node_bin", "python_bin"] },
  { title: "system.settings.groupDeploy", desc: "system.settings.groupDeployDesc", keys: ["port_pool"] },
  { title: "system.settings.groupQuota", desc: "system.settings.groupQuotaDesc", keys: ["quota_reqs", "quota_bytes"] },
];

const RESTART_KEYS = new Set<string>();

const PICKERS: Record<string, "dir" | "file"> = {
  node_bin: "file",
  python_bin: "file",
};

export function Settings() {
  const { t, tn } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const [settings, setSettings] = useState<Record<string, string>>({});
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [gh, setGh] = useState<GithubStatus | null>(null);
  const [params, setParams] = useSearchParams();
  const justInstalled = params.get("github") === "ok";

  async function load() {
    try {
      const s = await getSettings();
      setSettings(s);
      setDraft(s);
    } catch (e) {
      fail(e);
    }
  }

  useEffect(() => {
    load();
    getGithubStatus().then(setGh).catch(() => setGh(null));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("nav.settings")}
        description={t("system.settings.description")}
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{tn("system.settings.keys", Object.keys(settings).length)}</span>}
      />
      {justInstalled && (
        <div className="flex items-center justify-between gap-3 border border-ok/35 bg-ok/5 px-3 py-2">
          <p className="font-mono text-[10px] text-ok">
            {t("system.settings.githubConnected")}
          </p>
          <button
            type="button"
            onClick={() => setParams({}, { replace: true })}
            className="font-mono text-[9px] uppercase tracking-[0.08em] text-muted-foreground hover:text-foreground"
          >
            {t("common.close")}
          </button>
        </div>
      )}
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
      <McpCard />
      {GROUPS.map((group) => (
        <Section key={group.title} title={t(group.title)} description={t(group.desc)}>
          <div className="border border-border">
            {group.keys.filter((k) => k in settings).map((k, i) => {
              const v = settings[k];
              return (
                <div
                  key={k}
                  className="grid grid-cols-1 gap-2 border-b border-border/60 bg-card/35 px-3 py-2.5 last:border-0 md:grid-cols-[2rem_minmax(0,1fr)_18rem_6.5rem] md:items-center md:gap-3"
                >
                  <span className="folio hidden md:block">{String(i + 1).padStart(2, "0")}</span>
                  <div className="min-w-0">
                    <p className="font-mono text-[11px]">{t(LABELS[k] ?? k)}</p>
                    <p className="mt-0.5 font-mono text-[9px] text-muted-foreground/60">{k}</p>
                    {DESCRIPTION_KEYS[k] && <p className="mt-0.5 text-[10px] leading-relaxed text-muted-foreground">{t(DESCRIPTION_KEYS[k])}</p>}
                    {RESTART_KEYS.has(k) && <p className="mt-0.5 font-mono text-[9px] text-warn">{t("system.settings.restartNeeded")}</p>}
                  </div>
                  <span className="tnum min-w-0 truncate font-mono text-[11px] text-muted-foreground" title={v}>
                    {v === "" ? "—" : v}
                  </span>
                  {canWrite ? (
                    <div className="flex justify-end gap-1">
                      <FormDialog
                        trigger={
                          <Button variant="outline" size="xs">
                            {t("common.edit")}
                          </Button>
                        }
                        title={t("system.settings.editTitle", { key: k })}
                        description={DESCRIPTION_KEYS[k] ? t(DESCRIPTION_KEYS[k]) : t("system.settings.validatedHint")}
                        submitLabel={t("common.save")}
                        onSubmit={async () => {
                          await setSetting(k, draft[k] ?? v);
                          await load();
                        }}
                      >
                        <Field label={k} hint={PICKERS[k] ? t("system.settings.pathOnDisk") : undefined}>
                          {PICKERS[k] ? (
                            <PathInput
                              value={draft[k] ?? v}
                              onChange={(nv) => setDraft({ ...draft, [k]: nv })}
                              mode={PICKERS[k]}
                              ariaLabel={t("system.settings.valueAria", { key: k })}
                              className="tnum h-8 min-w-0 flex-1 font-mono text-[12px]"
                            />
                          ) : (
                            <Input
                              value={draft[k] ?? v}
                              onChange={(e) => setDraft({ ...draft, [k]: e.target.value })}
                              id={`set-${k}`}
                              aria-label={t("system.settings.valueAria", { key: k })}
                              className="tnum h-8 font-mono text-[12px]"
                              autoFocus
                            />
                          )}
                        </Field>
                      </FormDialog>
                      {v !== "" && (
                        <ConfirmButton
                          label={t("system.settings.reset")}
                          confirmLabel={t("common.confirm")}
                          title={t("system.settings.resetTitle", { key: k })}
                          onConfirm={() => {
                            setSetting(k, "").then(load).catch(fail);
                          }}
                        />
                      )}
                    </div>
                  ) : (
                    <span className="font-mono text-[9px] text-muted-foreground">{t("system.settings.readOnly")}</span>
                  )}
                </div>
              );
            })}
          </div>
        </Section>
      ))}
      <p className="font-mono text-[9px] leading-relaxed text-muted-foreground/70">
        {t("system.settings.secretsNote", { env: t("nav.environment") })}
      </p>
    </div>
  );
}

