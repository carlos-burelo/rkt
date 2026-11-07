import { Button } from "@/components/ui/button";
import { Dot, Avatar } from "../../shared/components/ui";
import { useUI } from "../../app/ui";
import { useFleet } from "../overview/hooks";
import { useI18n } from "../../shared/i18n";
import { useLogs } from "./logs.store";

// Pestaña de compilación: último despliegue + su log.
export function BuildTab() {
  const { t } = useI18n();
  const { deps } = useFleet();
  const appFilter = useUI((s) => s.appFilter);
  const build = useLogs((s) => s.build);
  const latest = deps.find((d) => appFilter === "" || d.app === appFilter);
  return (
    <div className="space-y-3 p-2">
      <div className="flex flex-wrap items-center gap-2 border border-border bg-card/35 px-2.5 py-2">
        {latest ? (
          <>
            <Dot status={latest.status} pulse={latest.status === "running"} />
            <Avatar name={latest.app} />
            <span className="tnum font-mono text-[10px]">
              #{latest.id} {latest.app} {latest.rev.slice(0, 8)} <span className="text-muted-foreground">({latest.status})</span>
            </span>
            <span className="flex-1" />
            <Button
              variant="outline"
              size="sm"
              onClick={() => useLogs.getState().fetchBuild(latest.id, t("common.loading"), t("ops.logs.noLog"))}
            >
              {t("ops.logs.loadBuild")}
            </Button>
          </>
        ) : (
          <span className="font-mono text-[10px] text-muted-foreground">
            {appFilter
              ? t("ops.logs.noDeploymentsApp", { app: appFilter })
              : t("ops.logs.noDeployments")}
          </span>
        )}
      </div>
      <div className="border border-border">
        <div className="flex items-center gap-2 border-b border-console-border bg-console-raised px-3 py-1.5">
          <span className="micro text-console-muted">{t("ops.logs.buildLog")}</span>
          <span className="flex-1" />
          <span className="tnum font-mono text-[9px] text-console-muted">
            {latest ? `#${latest.id}` : "—"}
          </span>
        </div>
        <pre className="console h-[58vh] overflow-auto p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap">
          {build}
        </pre>
      </div>
    </div>
  );
}
