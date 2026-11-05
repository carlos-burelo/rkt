import { useEffect, useState } from "react";
import { CalendarClock, Play, Plus, ScrollText } from "lucide-react";
import { toast } from "sonner";
import { CronJob, CronRun, cronRunLog, deleteCron, listCron, listCronRuns, runCron, saveCron } from "./api";
import { rel } from "../../shared/lib/time";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { Dot } from "../../shared/components/ui";
import { EmptyState, PageHeader, Stat, StatGrid } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";

function statusOf(job: CronJob): string {
  if (job.running) return "running";
  if (!job.last) return "idle";
  return job.last.status;
}

function statusTone(status: string): "ok" | "warn" | "bad" | "running" | "stopped" {
  switch (status) {
    case "ok": return "ok";
    case "running": return "running";
    case "failed": return "bad";
    case "timeout": return "warn";
    default: return "stopped";
  }
}

function CronForm({
  open, onOpenChange, job, apps, onSaved,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  job: CronJob | null;
  apps: string[];
  onSaved: () => void;
}) {
  const { t } = useI18n();
  const [app, setApp] = useState(job?.app ?? apps[0] ?? "");
  const [name, setName] = useState(job?.job ?? "");
  const [schedule, setSchedule] = useState(job?.schedule ?? "*/5 * * * *");
  const [command, setCommand] = useState(job?.command ?? "");
  const [timeout, setTimeout] = useState(job?.timeout ?? "10m");
  const [disabled, setDisabled] = useState(job?.disabled ?? false);

  useEffect(() => {
    if (!open) return;
    setApp(job?.app ?? apps[0] ?? "");
    setName(job?.job ?? "");
    setSchedule(job?.schedule ?? "*/5 * * * *");
    setCommand(job?.command ?? "");
    setTimeout(job?.timeout ?? "10m");
    setDisabled(job?.disabled ?? false);
  }, [open, job, apps]);

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      trigger={<button type="button" className="hidden" aria-hidden />}
      title={job ? t("cron.editTitle", { job: job.job }) : t("cron.newTitle")}
      description={t("cron.formDesc")}
      submitLabel={job ? t("common.save") : t("common.create")}
      canSubmit={app !== "" && name.trim() !== "" && schedule.trim() !== "" && command.trim() !== ""}
      onSubmit={async () => {
        await saveCron(app, name.trim(), {
          schedule: schedule.trim(),
          command: command.trim(),
          timeout: timeout.trim(),
          disabled,
        });
        toast.success(t("cron.saved"));
        onSaved();
      }}
    >
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field label={t("cron.app")}>
          <Select value={app} onValueChange={(v: string | null) => setApp(v ?? "")} disabled={!!job}>
            <SelectTrigger className="h-8 font-mono text-[12px]">
              <span>{app || t("cron.pickApp")}</span>
            </SelectTrigger>
            <SelectContent>
              {apps.map((a) => (
                <SelectItem key={a} value={a}>{a}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label={t("cron.name")} hint={t("cron.nameHint")}>
          <Input value={name} onChange={(e) => setName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ""))} className="h-8 font-mono text-[12px]" autoFocus={!job} disabled={!!job} />
        </Field>
      </div>
      <Field label={t("cron.schedule")} hint={t("cron.scheduleHint")}>
        <Input value={schedule} onChange={(e) => setSchedule(e.target.value)} className="h-8 font-mono text-[12px]" placeholder="*/5 * * * *" />
      </Field>
      <Field label={t("cron.command")} hint={t("cron.commandHint")}>
        <Input value={command} onChange={(e) => setCommand(e.target.value)} className="h-8 font-mono text-[12px]" placeholder="pnpm run cleanup" />
      </Field>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field label={t("cron.timeout")} hint={t("cron.timeoutHint")}>
          <Input value={timeout} onChange={(e) => setTimeout(e.target.value)} className="h-8 font-mono text-[12px]" placeholder="10m" />
        </Field>
        <label className="mt-6 flex h-8 w-fit items-center gap-2 border border-border bg-card/30 px-2.5 text-[11px] text-muted-foreground">
          <input type="checkbox" checked={disabled} onChange={(e) => setDisabled(e.target.checked)} />
          {t("cron.disabled")}
        </label>
      </div>
    </FormDialog>
  );
}

function RunsDialog({ job }: { job: CronJob }) {
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
        <pre className="max-h-56 overflow-auto border border-border bg-ink/40 p-2 font-mono text-[9px] leading-relaxed whitespace-pre-wrap">
          {log || t("cron.pickRun")}
        </pre>
      </div>
    </FormDialog>
  );
}

export function Cron() {
  const { t, tn } = useI18n();
  const { metas } = useFleet();
  const fail = useUI((u) => u.fail);
  const canWrite = useAuth((a) => a.canWrite);
  const [jobs, setJobs] = useState<CronJob[]>([]);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<CronJob | null>(null);
  const apps = metas.map((m) => m.name).sort();

  useEffect(() => {
    let alive = true;
    const load = () => listCron().then((j) => alive && setJobs(j)).catch(fail);
    load();
    const id = setInterval(load, 5000);
    return () => {
      alive = false;
      clearInterval(id);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
            <Button size="sm" onClick={() => { setEditing(null); setFormOpen(true); }}>
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
        {jobs.map((j) => {
          const status = statusOf(j);
          return (
            <div
              key={`${j.app}/${j.job}`}
              className="grid grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 px-3 py-2.5 last:border-0 lg:grid-cols-[1.5rem_minmax(0,1fr)_9rem_8rem_9rem_10rem] lg:gap-3"
            >
              <Dot status={statusTone(status)} />
              <div className="min-w-0">
                <p className="flex items-baseline gap-1.5 font-mono text-[12px]">
                  <span className="truncate">{j.job}</span>
                  <span className="shrink-0 text-[10px] text-muted-foreground">{j.app}</span>
                  {j.disabled && <span className="font-mono text-[9px] text-warn uppercase">{t("cron.disabledShort")}</span>}
                </p>
                <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">{j.command}</p>
              </div>
              <span className="tnum hidden font-mono text-[11px] text-muted-foreground lg:block">{j.schedule}</span>
              <span className="tnum hidden font-mono text-[10px] text-muted-foreground lg:block">
                {j.next ? j.next.slice(5, 16).replace("T", " ") : "—"}
              </span>
              <span className={`tnum hidden text-right font-mono text-[10px] lg:block ${j.last ? (j.last.status === "ok" ? "text-ok" : j.last.status === "failed" ? "text-bad" : "text-warn") : "text-muted-foreground"}`}>
                {j.last ? `${j.last.status} · ${rel(j.last.started_at)}` : t("cron.never")}
              </span>
              <div className="flex justify-end gap-0.5">
                {canWrite && !j.disabled && (
                  <Button
                    variant="ghost"
                    size="xs"
                    title={t("cron.runNow")}
                    onClick={() => runCron(j.app, j.job).then(() => toast.success(t("cron.started", { job: j.job }))).catch(fail)}
                  >
                    <Play className="h-3 w-3" />
                  </Button>
                )}
                <RunsDialog job={j} />
                {canWrite && (
                  <>
                    <Button variant="ghost" size="xs" title={t("common.edit")} onClick={() => { setEditing(j); setFormOpen(true); }}>
                      <CalendarClock className="h-3 w-3" />
                    </Button>
                    <ConfirmButton
                      title={t("cron.deleteTitle", { job: j.job })}
                      onConfirm={() => deleteCron(j.app, j.job).then(() => listCron().then(setJobs)).catch(fail)}
                    />
                  </>
                )}
              </div>
            </div>
          );
        })}
        {jobs.length === 0 && <EmptyState title={t("cron.emptyTitle")} hint={t("cron.emptyHint")} />}
      </div>
      <CronForm
        open={formOpen}
        onOpenChange={(v) => { setFormOpen(v); if (!v) setEditing(null); }}
        job={editing}
        apps={apps}
        onSaved={() => listCron().then(setJobs).catch(fail)}
      />
    </div>
  );
}
