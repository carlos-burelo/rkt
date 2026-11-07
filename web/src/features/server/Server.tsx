import { useEffect } from "react";
import { EmptyState, PageHeader } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import { useServer } from "./server.store";
import { ServerOverview } from "./ServerOverview";
import { ServerResources } from "./ServerResources";
import { ServerSystem } from "./ServerSystem";
import { uptime } from "./format";

// Servidor: composición. Datos en el store, piezas presentacionales aparte.
export function Server() {
  const { t } = useI18n();
  const snap = useServer((s) => s.snap);
  const history = useServer((s) => s.history);
  const procs = useServer((s) => s.procs);
  const err = useServer((s) => s.err);

  useEffect(() => {
    const st = useServer.getState();
    st.start();
    return () => st.stop();
  }, []);

  if (!snap && err === "") {
    return <p className="font-mono text-[11px] text-muted-foreground">{t("server.loading")}</p>;
  }
  if (err !== "") {
    return <EmptyState title={t("server.unavailable")} hint={err} />;
  }
  if (!snap) return null;

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("nav.server")}
        description={t("server.description")}
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {snap.hostname} · {snap.os} · {uptime(snap.uptime_s, t)}
          </span>
        }
      />
      <ServerOverview snap={snap} history={history} />
      <ServerResources snap={snap} />
      <ServerSystem snap={snap} procs={procs} />
    </div>
  );
}
