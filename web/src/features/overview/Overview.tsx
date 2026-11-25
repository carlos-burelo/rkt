import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { PageHeader, Section } from "../../shared/components/patterns";
import { MicroLink } from "../../shared/components/ui";
import { useFleet } from "./hooks";
import { useI18n } from "../../shared/i18n";
import { fetchProxy } from "../proxy/api";
import { Metrics } from "./Metrics";
import { FirstRun } from "./FirstRun";
import { Activity } from "./Departures";
import { AttentionBanner, RestartBanner, TlsBanner } from "./Banners";
import { UnitsTable, OverviewSide } from "./UnitsTable";

// Resumen: composición. Datos del bus, piezas autocontenidas.
export function Overview() {
  const { t } = useI18n();
  const { status, evs, deps, metas, loading } = useFleet();
  const navigate = useNavigate();
  const apps = status?.apps ?? [];
  const openApp = (name: string) => navigate(`/app/${name}`);
  const [tlsSoon, setTlsSoon] = useState<{ host: string; date: string }[]>([]);

  useEffect(() => {
    let alive = true;
    fetchProxy()
      .then((rows) => {
        if (!alive) return;
        const soon: { host: string; date: string }[] = [];
        const now = Date.now();
        for (const r of rows) {
          r.hosts.forEach((host, i) => {
            const tls = r.tls[i] ?? "off";
            if (tls === "off" || tls === "auto") return;
            const d = new Date(tls).getTime();
            if (Number.isNaN(d)) return;
            if (d - now <= 14 * 86400000) soon.push({ host, date: tls });
          });
        }
        setTlsSoon(soon);
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  if (loading)
    return <p className="font-mono text-[11px] text-muted-foreground">{t("overview.loading")}</p>;

  const discovered = evs.filter((e) => e.kind === "discover").slice(0, 2);
  const openAlerts = evs.filter((e) => e.kind === "alert").slice(0, 3);

  return (
    <div className="space-y-5">
      <PageHeader
        title={t("nav.overview")}
        description={t("overview.description")}
      />

      <AttentionBanner apps={apps} />
      <RestartBanner fields={status?.pending_restart ?? []} />
      <TlsBanner items={tlsSoon} />

      {apps.length === 0 && <FirstRun apps={apps} metas={metas} deps={deps} />}
      <Metrics apps={apps} deps={deps} />

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_330px]">
        <div className="min-w-0 space-y-5">
          <UnitsTable apps={apps} metas={metas} deps={deps} status={status} />
          <Section
            title={t("overview.sections.events")}
            description={t("overview.sections.eventsDesc")}
            collapsible
            defaultOpen={false}
            actions={
              <span className="flex items-center gap-2">
                <span className="tnum font-mono text-[10px] text-muted-foreground">{evs.length}</span>
                <MicroLink to="/observability" className="text-[10px]">{t("overview.sections.seeAll")}</MicroLink>
              </span>
            }
          >
            <Activity evs={evs} />
          </Section>
        </div>

        <OverviewSide
          deps={deps}
          openAlerts={openAlerts}
          discovered={discovered}
          openApp={openApp}
        />
      </div>
    </div>
  );
}
