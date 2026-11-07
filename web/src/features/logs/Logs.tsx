import { useEffect } from "react";
import { PageHeader } from "../../shared/components/patterns";
import { Dot } from "../../shared/components/ui";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import { useLogs } from "./logs.store";
import { useLogLines } from "./useLogLines";
import { LogToolbar } from "./LogToolbar";
import { LogOptions } from "./LogOptions";
import { LogConsole } from "./LogConsole";
import { BuildTab } from "./BuildTab";

// Consola: composición. Estado en el store, piezas aparte.
export function Logs() {
  const { t } = useI18n();
  const appFilter = useUI((s) => s.appFilter);
  const online = useUI((s) => s.online);
  const tab = useLogs((s) => s.tab);
  const live = useLogs((s) => s.live);
  const streamState = useLogs((s) => s.streamState);
  const log = useLogs((s) => s.log);
  const { lines, rendered } = useLogLines();

  useEffect(() => {
    useLogs.getState().load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [appFilter, online]);

  useEffect(() => {
    useLogs.getState().resetStream();
  }, [appFilter, online, live, tab]);

  useEffect(() => {
    if (!live || tab !== "runtime" || !online) return;
    return useLogs.getState().connect();
  }, [live, tab, appFilter, online]);

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("ops.logs.title")}
        description={t("ops.logs.description")}
        meta={
          <span className="tnum inline-flex items-center gap-2 font-mono text-[10px] text-muted-foreground">
            <Dot status={tab === "runtime" && live ? (streamState === "retrying" ? "degraded" : "running") : "stopped"} pulse={tab === "runtime" && live} />
            {tab === "runtime"
              ? live
                ? streamState === "retrying"
                  ? t("ops.logs.meta.retry")
                  : t("ops.logs.meta.live")
                : t("ops.logs.meta.paused")
              : t("ops.logs.meta.build")}
            {appFilter ? ` · ${appFilter}` : t("ops.logs.meta.operational")}
          </span>
        }
      />

      <div className="border border-border bg-card/35">
        <LogToolbar lines={lines} />

        {tab === "runtime" && (
          <div className="space-y-3 p-2">
            <LogOptions lines={lines} total={log === "" ? 0 : log.split("\n").length} />
            <LogConsole rendered={rendered} empty={lines.vis.length === 0} />
          </div>
        )}

        {tab === "build" && <BuildTab />}
      </div>
    </div>
  );
}
