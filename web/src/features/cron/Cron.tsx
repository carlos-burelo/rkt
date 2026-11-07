import { useEffect } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState, PageHeader, Stat, StatGrid } from "../../shared/components/patterns";
import { useFleet } from "../overview/hooks";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import { useCron } from "./cron.store";
import { CronForm } from "./CronForm";
import { CronRow } from "./CronRow";

// Cron: layout + tabla. Estado y poll en el store, piezas aparte.
export function Cron() {
  const { t, tn } = useI18n();
  const { metas } = useFleet();
  const jobs = useCron((s) => s.jobs);
  const formOpen = useCron((s) => s.formOpen);
  const editing = useCron((s) => s.editing);
  const canWrite = useAuth((a) => a.canWrite);
  const apps = metas.map((m) => m.name).sort();

  useEffect(() => {
    const st = useCron.getState();
    st.start();
    return () => st.stop();
  }, []);

  const total = jobs.length;
  const active = jobs.filter((j) => !j.disabled).length;
  const running = jobs.filter((j) => j.running).length;
  const failed = jobs.filter((j) => j.last?.status === "failed" || j.last?.status === "timeout").length;

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("nav.cron")}
        description={t("cron.description")}
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{tn("cron.count", total)}</span>}
        actions={
          canWrite ? (
            <Button size="sm" onClick={() => useCron.getState().openNew()}>
              <Plus className="h-3 w-3" /> {t("cron.new")}
            </Button>
          ) : undefined
        }
      />
      <StatGrid>
        <Stat value={`${total}`} label={t("cron.total")} sub={t("cron.totalSub")} />
        <Stat value={`${active}`} label={t("cron.active")} sub={t("cron.activeSub")} tone={active > 0 ? "ok" : undefined} />
        <Stat value={`${running}`} label={t("cron.running")} sub={t("cron.runningSub")} tone={running > 0 ? "signal" : undefined} />
        <Stat value={`${failed}`} label={t("cron.failed")} sub={t("cron.failedSub")} tone={failed > 0 ? "bad" : undefined} />
      </StatGrid>
      <div className="border border-border" role="table" aria-label={t("cron.tableAria")}>
        <div className="hidden grid-cols-[1.5rem_minmax(0,1fr)_9rem_8rem_9rem_10rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 lg:grid" aria-hidden>
          <span className="micro">{t("cron.col.state")}</span>
          <span className="micro">{t("cron.col.job")}</span>
          <span className="micro">{t("cron.col.schedule")}</span>
          <span className="micro">{t("cron.col.next")}</span>
          <span className="micro text-right">{t("cron.col.last")}</span>
          <span />
        </div>
        {jobs.map((j) => (
          <CronRow key={`${j.app}/${j.job}`} job={j} />
        ))}
        {jobs.length === 0 && <EmptyState title={t("cron.emptyTitle")} hint={t("cron.emptyHint")} />}
      </div>
      <CronForm open={formOpen} job={editing} apps={apps} />
    </div>
  );
}
