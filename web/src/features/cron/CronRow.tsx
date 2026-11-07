import { CalendarClock, Play } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { Dot } from "../../shared/components/ui";
import { RowMenu } from "../../shared/components/menus";
import { rel } from "../../shared/lib/time";
import { useAuth } from "../auth/store";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import type { CronJob } from "./api";
import { useCron } from "./cron.store";
import { RunsDialog } from "./RunsDialog";
import { cronStatusOf, cronTone } from "./status";

import { cronStatusOf as statusOf, cronTone as statusTone } from "./status";

// Fila de tarea: acciones finas contra el store.
export function CronRow({ job }: { job: CronJob }) {
  const { t } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const remove = useCron((s) => s.remove);
  const runNow = useCron((s) => s.runNow);
  const openEdit = useCron((s) => s.openEdit);
  const status = statusOf(job);
  return (
    <div className="grid grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 px-3 py-2.5 last:border-0 lg:grid-cols-[1.5rem_minmax(0,1fr)_9rem_8rem_9rem_10rem] lg:gap-3">
      <Dot status={statusTone(status)} />
      <div className="min-w-0">
        <p className="flex items-baseline gap-1.5 font-mono text-[12px]">
          <span className="truncate">{job.job}</span>
          <span className="shrink-0 text-[10px] text-muted-foreground">{job.app}</span>
          {job.disabled && <span className="font-mono text-[9px] text-warn uppercase">{t("cron.disabledShort")}</span>}
        </p>
        <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
          {job.type === "http" ? `${job.method || "GET"} ${job.path || ""}` : job.command}
        </p>
      </div>
      <span className="tnum hidden font-mono text-[11px] text-muted-foreground lg:block">{job.schedule}</span>
      <span className="tnum hidden font-mono text-[10px] text-muted-foreground lg:block">
        {job.next ? job.next.slice(5, 16).replace("T", " ") : "—"}
      </span>
      <span className={`tnum hidden text-right font-mono text-[10px] lg:block ${job.last ? (job.last.status === "ok" ? "text-ok" : job.last.status === "failed" ? "text-bad" : "text-warn") : "text-muted-foreground"}`}>
        {job.last ? `${job.last.status} · ${rel(job.last.started_at)}` : t("cron.never")}
      </span>
      <div className="flex justify-end gap-0.5">
        {canWrite && !job.disabled && (
          <Button
            variant="ghost"
            size="xs"
            title={t("cron.runNow")}
            onClick={() => runNow(job.app, job.job).then(() => toast.success(t("cron.started", { job: job.job }))).catch(fail)}
          >
            <Play className="h-3 w-3" />
          </Button>
        )}
        {canWrite ? (
          <RowMenu label={t("cron.rowActions", { job: job.job })}>
            <RunsDialog job={job} trigger={<DropdownMenuItem>{t("cron.history")}</DropdownMenuItem>} />
            <DropdownMenuItem onClick={() => openEdit(job)}>
              <CalendarClock className="h-3 w-3" /> {t("common.edit")}
            </DropdownMenuItem>
            <ConfirmButton
              title={t("cron.deleteTitle", { job: job.job })}
              onConfirm={() => remove(job.app, job.job)}
            />
          </RowMenu>
        ) : (
          <RunsDialog job={job} />
        )}
      </div>
    </div>
  );
}
