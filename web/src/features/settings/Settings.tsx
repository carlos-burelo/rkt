import { useEffect } from "react";
import { useSearchParams } from "react-router";
import { PageHeader, Section } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import { McpCard } from "./McpCard";
import { GithubCard } from "./GithubCard";
import { SettingRow } from "./SettingRow";
import { useSettings } from "./settings.store";

const GROUPS: { title: string; desc: string; keys: string[] }[] = [
  { title: "system.settings.groupNetwork", desc: "system.settings.groupNetworkDesc", keys: ["base_domain", "public_ip", "site_name"] },
  { title: "system.settings.groupProcess", desc: "system.settings.groupProcessDesc", keys: ["node_bin", "python_bin"] },
  { title: "system.settings.groupDeploy", desc: "system.settings.groupDeployDesc", keys: ["port_pool"] },
  { title: "system.settings.groupQuota", desc: "system.settings.groupQuotaDesc", keys: ["quota_reqs", "quota_bytes"] },
];

// Ajustes: layout + grupos. Estado en el store, piezas aparte.
export function Settings() {
  const { t, tn } = useI18n();
  const settings = useSettings((s) => s.settings);
  const [params, setParams] = useSearchParams();
  const justInstalled = params.get("github") === "ok";

  useEffect(() => {
    useSettings.getState().load();
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
      <GithubCard />
      <McpCard />
      {GROUPS.map((group) => (
        <Section key={group.title} title={t(group.title)} description={t(group.desc)}>
          <div className="border border-border">
            {group.keys.filter((k) => k in settings).map((k, i) => (
              <SettingRow key={k} k={k} index={i} />
            ))}
          </div>
        </Section>
      ))}
      <p className="font-mono text-[9px] leading-relaxed text-muted-foreground/70">
        {t("system.settings.secretsNote", { env: t("nav.environment") })}
      </p>
    </div>
  );
}
