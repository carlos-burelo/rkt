import { useEffect, useState } from "react";
import { ScrollText } from "lucide-react";
import { Button } from "@/components/ui/button";
import { FormDialog } from "../../shared/components/FormDialog";
import { Dot } from "../../shared/components/ui";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import { cronRunLog, listCronRuns, type CronJob, type CronRun } from "./api";

function statusTone(status: string): "ok" | "warn" | "bad" | "running" | "stopped" {
  switch (status) {
    case "ok": return "ok";
    case "running": return "running";
    case "failed": return "bad";
    case "timeout": return "warn";
    default: return "stopped";
  }
}

// Historial de ejecuciones de un job: instancia local por fila.
export function RunsDialog({ job }: { job: CronJob }) {
  const { t } = useI18n();
  const fail = useUI((u) => u.fail);
  const [open, setOpen] = useState(false);
  const [runs, setRuns] = useState<CronRun[]>([]);
  const [log, setLog] = useState("");
  const [selected, setSelected] = useState<number | null>(null);

  useEffect(() => {
    if (!open) return;
    listCronRuns(job.app, job.job, 30).then(setRuns).catch(fail);
    setLog("");
    setSelected(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, job.app, job.job]);

  return (
    <FormDialog
      open={open}
      onOpenChange={setOpen}
      trigger={
        <Button variant="ghost" size="xs" title={t("cron.history")}>
          <ScrollText className="h-3 w-3" />
        </Button>
      }
      title={t("cron.historyTitle", { app: job.app, job: job.job })}
      description={t("cron.historyDesc")}
      submitLabel={t("common.close")}
      onSubmit={() => setOpen(false)}
    >
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        <div className="max-h-56 overflow-auto border border-border">
          {runs.map((r) => (
            <button
              key={r.id}
              type="button"
              onClick={() => {
                setSelected(r.id);
                cronRunLog(r.id).then(setLog).catch(() => setLog(t("cron.noLog")));
              }}
              className={`flex w-full items-center gap-2 border-b border-border/60 px-2 py-1.5 text-left last:border-0 hover:bg-accent/40 ${selected === r.id ? "bg-secondary/40" : ""}`}
            >
              <Dot status={statusTone(r.status)} />
              <span className="tnum font-mono text-[9px] text-muted-foreground">{r.started_at.slice(5, 16).replace("T", " ")}</span>
              <span className={`ml-auto font-mono text-[9px] uppercase ${r.status === "ok" ? "text-ok" : r.status === "failed" ? "text-bad" : "text-warn"}`}>{r.status}</span>
            </button>
          ))}
          {runs.length === 0 && <p className="px-2 py-4 font-mono text-[10px] text-muted-foreground">{t("cron.noRuns")}</p>}
        </div>
        <pre className="console max-h-56 overflow-auto border border-console-border p-2 font-mono text-[9px] leading-relaxed whitespace-pre-wrap">
          {log || t("cron.pickRun")}
        </pre>
      </div>
    </FormDialog>
  );
}
