import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Proxy } from "../proxy/Proxy";
import { DomainDrawer } from "./DomainDrawer";
import { FunnelMap } from "./FunnelMap";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
import { useAuth } from "../auth/store";
import { LocalZones } from "./LocalZones";
import { useFleet } from "../overview/hooks";
import { useProxy } from "../proxy/proxy.store";
import { buildLinks } from "../proxy/graph";
import { useRoutes } from "./routes.store";
import { RouteRow, NewRouteDialog, routeAddr } from "./RouteParts";
import { useI18n } from "../../shared/i18n";

// Rutas: layout + tabs. Estado en stores, piezas aparte.
export function Routes() {
  const { t, tn } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const { status } = useFleet();
  const proxyRows = useProxy((s) => s.rows);
  const routes = useRoutes((s) => s.routes);
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") === "diagnostico" ? "diagnostico" : "rutas";
  const [selectedHost, setSelectedHost] = useState<string | null>(null);

  useEffect(() => {
    useRoutes.getState().load();
    useProxy.getState().load(false);
  }, []);

  const mapLinks =
    proxyRows?.length
      ? buildLinks(proxyRows)
      : routes.flatMap((r) =>
          (r.hosts.length ? r.hosts : ["∗"]).map((h) => ({
            host: r.hosts.length ? h : routeAddr(r),
            target: r.target,
            short: r.target.replace(/^https?:\/\//, "").replace(/^127\.0\.0\.1/, "local"),
            ok: true,
            tls: "off",
            stale: true,
          })),
        );

  const hostCount = routes.reduce((n, r) => n + r.hosts.length, 0);
  const reloadAll = () => {
    useRoutes.getState().load();
    useProxy.getState().load(true);
  };

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("nav.routes")}
        description={t("serve.routes.desc")}
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {tn("serve.routes.count", routes.length)} · {tn("serve.hosts.count", hostCount)}
          </span>
        }
        actions={canWrite && tab === "rutas" ? <NewRouteDialog onAdded={reloadAll} /> : undefined}
      />
      <Tabs value={tab} onValueChange={(v) => setParams(v === "diagnostico" ? { tab: "diagnostico" } : {}, { replace: true })}>
        <TabsList className="w-max">
          <TabsTrigger value="rutas">{t("serve.routes.tab")}</TabsTrigger>
          <TabsTrigger value="diagnostico">{t("serve.routes.diagnostics")}</TabsTrigger>
        </TabsList>
      </Tabs>

      {tab === "diagnostico" && <Proxy embedded />}

      {tab === "rutas" && (
        <>
          <LocalZones onChanged={reloadAll} />
          <FunnelMap links={mapLinks} onAdd={() => useRoutes.getState().setAddOpen(true)} />
          <div className="border border-border" role="table" aria-label={t("serve.routes.tableAria")}>
            <div
              className="hidden grid-cols-[1.5rem_minmax(0,1fr)_auto_1.5rem_minmax(0,1.4fr)_auto] items-center gap-2 border-b border-border bg-secondary/25 px-3 py-1.5 md:grid"
              aria-hidden
            >
              <span className="micro">{t("serve.field.stateShort")}</span>
              <span className="micro">{t("serve.field.host")}</span>
              <span className="micro">{t("serve.field.tls")}</span>
              <span />
              <span className="micro">{t("serve.field.target")}</span>
              <span />
            </div>
            {routes.map((r) => (
              <RouteRow
                key={(r.hosts.join(",") || "∗") + (r.path ?? "") + r.target}
                r={r}
                status={status}
                onOpen={() => setSelectedHost(r.hosts[0] ?? null)}
              />
            ))}
            {routes.length === 0 && (
              canWrite ? (
                <div className="space-y-3 px-4 py-6">
                  <p className="micro">{t("serve.routes.guideTitle")}</p>
                  <ol className="space-y-2">
                    {[
                      t("serve.routes.guide1"),
                      t("serve.routes.guide2"),
                      t("serve.routes.guide3"),
                    ].map((step, i) => (
                      <li key={i} className="flex items-baseline gap-2.5">
                        <span className="tnum font-mono text-[10px] text-signal">{String(i + 1).padStart(2, "0")}</span>
                        <span className="font-mono text-[11px] text-muted-foreground">{step}</span>
                      </li>
                    ))}
                  </ol>
                </div>
              ) : (
                <EmptyState title={t("serve.routes.emptyTitle")} hint={t("serve.routes.emptyHint")} />
              )
            )}
          </div>
        </>
      )}
      <DomainDrawer host={selectedHost} onClose={() => setSelectedHost(null)} />
    </div>
  );
}
