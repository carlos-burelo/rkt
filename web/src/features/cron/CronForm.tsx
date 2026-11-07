import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useI18n } from "../../shared/i18n";
import { saveCron, type CronJob, type CronType } from "./api";
import { useCron } from "./cron.store";

// Formulario de alta/edición (estado local: cada apertura parte del job).
export function CronForm({
  open, job, apps,
}: {
  open: boolean;
  job: CronJob | null;
  apps: string[];
}) {
  const { t } = useI18n();
  const onOpenChange = useCron((s) => s.closeForm);  const load = useCron((s) => s.load);
  const [app, setApp] = useState(job?.app ?? apps[0] ?? "");
  const [name, setName] = useState(job?.job ?? "");
  const [schedule, setSchedule] = useState(job?.schedule ?? "*/5 * * * *");
  const [type, setType] = useState<CronType>(job?.type === "http" ? "http" : "command");
  const [command, setCommand] = useState(job?.command ?? "");
  const [path, setPath] = useState(job?.path ?? "");
  const [method, setMethod] = useState(job?.method === "POST" ? "POST" : "GET");
  const [timeout, setTimeout] = useState(job?.timeout ?? "10m");
  const [disabled, setDisabled] = useState(job?.disabled ?? false);

  useEffect(() => {
    if (!open) return;
    setApp(job?.app ?? apps[0] ?? "");
    setName(job?.job ?? "");
    setSchedule(job?.schedule ?? "*/5 * * * *");
    setType(job?.type === "http" ? "http" : "command");
    setCommand(job?.command ?? "");
    setPath(job?.path ?? "");
    setMethod(job?.method === "POST" ? "POST" : "GET");
    setTimeout(job?.timeout ?? "10m");
    setDisabled(job?.disabled ?? false);
  }, [open, job, apps]);

  const validTarget = type === "http" ? path.trim().startsWith("/") : command.trim() !== "";

  return (
    <FormDialog
      open={open}
      onOpenChange={(v) => {
        if (!v) onOpenChange();
      }}
      trigger={<button type="button" className="hidden" aria-hidden />}
      title={job ? t("cron.editTitle", { job: job.job }) : t("cron.newTitle")}
      description={t("cron.formDesc")}
      submitLabel={job ? t("common.save") : t("common.create")}
      canSubmit={app !== "" && name.trim() !== "" && schedule.trim() !== "" && validTarget}
      onSubmit={async () => {
        await saveCron(app, name.trim(), {
          schedule: schedule.trim(),
          type,
          command: type === "command" ? command.trim() : "",
          path: type === "http" ? path.trim() : "",
          method: type === "http" ? method : "",
          timeout: timeout.trim(),
          disabled,
        });
        toast.success(t("cron.saved"));
        onOpenChange();
        load();
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
      <Field label={t("cron.type")} hint={t("cron.typeHint")}>
        <Select value={type} onValueChange={(v: string | null) => setType(v === "http" ? "http" : "command")}>
          <SelectTrigger className="h-8 font-mono text-[12px]">
            <span>{type === "http" ? t("cron.typeHttp") : t("cron.typeCommand")}</span>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="command">{t("cron.typeCommand")}</SelectItem>
            <SelectItem value="http">{t("cron.typeHttp")}</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      {type === "command" ? (
        <Field label={t("cron.command")} hint={t("cron.commandHint")}>
          <Input value={command} onChange={(e) => setCommand(e.target.value)} className="h-8 font-mono text-[12px]" placeholder="pnpm run cleanup" />
        </Field>
      ) : (
        <div className="grid grid-cols-[minmax(0,1fr)_7rem] gap-3">
          <Field label={t("cron.path")} hint={t("cron.pathHint")}>
            <Input value={path} onChange={(e) => setPath(e.target.value)} className="h-8 font-mono text-[12px]" placeholder="/api/cron" />
          </Field>
          <Field label={t("cron.method")}>
            <Select value={method} onValueChange={(v: string | null) => setMethod(v === "POST" ? "POST" : "GET")}>
              <SelectTrigger className="h-8 font-mono text-[12px]">
                <span>{method}</span>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="GET">GET</SelectItem>
                <SelectItem value="POST">POST</SelectItem>
              </SelectContent>
            </Select>
          </Field>
        </div>
      )}
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
