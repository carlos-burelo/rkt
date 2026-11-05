import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import { Pencil, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Section } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { PathInput } from "../../shared/components/PathPicker";
import { DomainSuggest } from "../../shared/components/DomainSuggest";
import { updateApp, deleteApp } from "./api";
import { canSaveApp, normalizeDomain } from "./model";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";

type TextKey = "command" | "args" | "dir" | "static" | "env_file" | "restart_delay" | "min_uptime" | "repo" | "ref" | "health_path";

// Ajustes por app: vista de solo lectura + diálogos de edición/borrado.
// Un solo guardado aplica la configuración; el supervisor reinicia lo cambiado.
export function AppSettings() {
  const { t } = useI18n();
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const { metas } = useFleet();
  const fail = useUI((u) => u.fail);
  const { canWrite } = useAuth();
  const meta = metas.find((m) => m.name === name);
  const external = !!meta?.external;
  const [form, setForm] = useState({
    command: "",
    args: "",
    dir: "",
    static: "",
    env_file: "",
    port: "",
    domains: [] as string[],
    memory_mb: "",
    restart_delay: "",
    min_uptime: "",
    health_path: "",
    disabled: false,
    repo: "",
    ref: "",
    build: "",
    auto: false,
  });
  const [domainDraft, setDomainDraft] = useState("");
  const [confirmDel, setConfirmDel] = useState("");

  useEffect(() => {
    setForm({
      command: meta?.command ?? "",
      args: (meta?.args ?? []).join(" "),
      dir: meta?.dir ?? "",
      static: meta?.static ?? "",
      env_file: meta?.env_file ?? "",
      port: meta?.port ?? "",
      domains: meta?.domains ?? [],
      memory_mb: meta?.memory_mb ? String(meta.memory_mb) : "",
      restart_delay: meta?.restart_delay ?? "",
      min_uptime: meta?.min_uptime ?? "",
      health_path: meta?.health_path ?? "/",
      disabled: !!meta?.disabled,
      repo: meta?.deploy?.repo ?? "",
      ref: meta?.deploy?.ref ?? "",
      build: (meta?.deploy?.build ?? []).join("\n"),
      auto: meta?.deploy?.auto ?? false,
    });
  }, [meta]);

  const set = (k: TextKey | "port" | "memory_mb", v: string) => setForm((f) => ({ ...f, [k]: v }));

  const addDomain = (raw: string) => {
    const host = normalizeDomain(raw);
    if (!host) return;
    setForm((f) => (f.domains.includes(host) ? f : { ...f, domains: [...f.domains, host] }));
    setDomainDraft("");
  };
  const removeDomain = (host: string) => setForm((f) => ({ ...f, domains: f.domains.filter((h) => h !== host) }));

  async function save() {
    if (external) {
      await updateApp(name, {
        dir: form.dir || undefined,
        env_file: form.env_file || "",
        port: form.port.trim() === "" ? undefined : Number(form.port),
        domains: form.domains,
        memory_mb: form.memory_mb.trim() === "" ? undefined : Number(form.memory_mb),
        disabled: form.disabled,
      });
      return;
    }
    await updateApp(name, {
      command: form.command || undefined,
      args: form.args.trim() === "" ? [] : form.args.trim().split(/\s+/),
      dir: form.dir || undefined,
      env_file: form.env_file || "",
      static: form.static || "",
      port: form.port.trim() === "" ? undefined : Number(form.port),
      domains: form.domains,
      restart_delay: form.restart_delay || undefined,
      min_uptime: form.min_uptime || undefined,
      health_path: form.health_path || "/",
      memory_mb: form.memory_mb.trim() === "" ? undefined : Number(form.memory_mb),
      disabled: form.disabled,
      deploy: form.repo
        ? { repo: form.repo, ref: form.ref || "main", build: form.build.split("\n").map((s) => s.trim()).filter(Boolean), auto: form.auto }
        : null,
    });
  }

  function EditField({
    label,
    hint,
    value,
    onChange,
    pick,
    placeholder,
  }: {
    label: string;
    hint: string;
    value: string;
    onChange: (v: string) => void;
    pick?: "dir" | "file";
    placeholder?: string;
  }) {
    return (
      <Field label={label} hint={hint}>
        {pick ? (
          <PathInput
            value={value}
            onChange={onChange}
            mode={pick}
            ariaLabel={label}
            className="h-8 min-w-0 flex-1 font-mono text-[12px]"
          />
        ) : (
          <Input
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder={placeholder}
            className="h-8 font-mono text-[12px]"
          />
        )}
      </Field>
    );
  }

  const Row = ({ label, value }: { label: string; value?: string }) => (
    <div className="flex items-baseline justify-between gap-3 border-b border-border/40 py-1.5 last:border-0">
      <span className="micro shrink-0">{label}</span>
      <span className="min-w-0 truncate text-right font-mono text-[11px]" title={value || undefined}>
        {value && value !== "" ? value : "—"}
      </span>
    </div>
  );

  return (
    <div className="space-y-4">
      <Section
        title={t("nav.settings")}
        description={t("apps.settings.desc")}
        actions={
          canWrite ? (
            <div className="flex items-center gap-2">
              <FormDialog
                trigger={
                  <Button variant="outline" size="sm">
                    <Pencil className="h-3 w-3" /> {t("common.edit")}
                  </Button>
                }
                title={t("apps.settings.editTitle", { name })}
                description={t("apps.settings.editDesc")}
                submitLabel={t("common.save")}
                canSubmit={canSaveApp(form.command, external, meta?.static)}
                onSubmit={save}
                className="sm:max-w-2xl"
              >
                <div className="grid grid-cols-1 gap-x-3 gap-y-2.5 md:grid-cols-2">
                  <div className="field-label md:col-span-2">
                    <span className="flex items-baseline justify-between gap-2">
                      <span>{t("apps.settings.domains")}</span>
                      <span className="normal-case tracking-normal text-muted-foreground/55">
                        {t("apps.settings.domainsHint")}
                      </span>
                    </span>
                    <div className="space-y-1.5">
                      {form.domains.map((host) => (
                        <div key={host} className="flex items-center gap-2 border border-border bg-card/30 px-2 py-1">
                          <span className="min-w-0 flex-1 truncate font-mono text-[12px]">{host}</span>
                          <Button type="button" variant="destructive" size="xs" onClick={() => removeDomain(host)}>
                            {t("common.remove")}
                          </Button>
                        </div>
                      ))}
                      <div className="flex items-center gap-2">
                        <Input
                          value={domainDraft}
                          onChange={(e) => setDomainDraft(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === "Enter") {
                              e.preventDefault();
                              addDomain(domainDraft);
                            }
                          }}
                          placeholder="app.localhost"
                          className="h-8 min-w-0 flex-1 font-mono text-[12px]"
                        />
                        <Button type="button" variant="outline" size="xs" disabled={domainDraft.trim() === ""} onClick={() => addDomain(domainDraft)}>
                          {t("common.add")}
                        </Button>
                      </div>
                    </div>
                    <DomainSuggest name={meta?.name ?? name} onPick={addDomain} />
                  </div>
                  <EditField
                    label={t("apps.field.port")}
                    hint={t("apps.settings.portHint")}
                    value={form.port}
                    onChange={(v) => set("port", v.replace(/\D/g, ""))}
                    placeholder="3001"
                  />
                  <EditField
                    label={t("apps.field.memory")}
                    hint={t("apps.settings.memoryHint")}
                    value={form.memory_mb}
                    onChange={(v) => set("memory_mb", v.replace(/\D/g, ""))}
                    placeholder={t("apps.settings.noLimit")}
                  />
                  {!external && (
                    <>
                      <EditField label={t("apps.field.command")} hint={t("apps.settings.commandHint")} value={form.command} onChange={(v) => set("command", v)} pick="file" />
                      <EditField
                        label={t("apps.field.args")}
                        hint={t("apps.settings.argsHint")}
                        value={form.args}
                        onChange={(v) => set("args", v)}
                        placeholder="server.js"
                      />
                    </>
                  )}
                  <EditField label={t("apps.field.dir")} hint={t("apps.settings.dirHint")} value={form.dir} onChange={(v) => set("dir", v)} pick="dir" />
                  {!external && (
                    <EditField
                      label={t("apps.field.static")}
                      hint={t("apps.settings.staticHint")}
                      value={form.static}
                      onChange={(v) => set("static", v)}
                      pick="dir"
                    />
                  )}
                  <EditField label={t("apps.field.envFile")} hint={t("apps.settings.envFileHint")} value={form.env_file} onChange={(v) => set("env_file", v)} pick="file" />
                  {!external && (
                    <>
                      <EditField label={t("apps.field.restart")} hint={t("apps.settings.restartDelayHint")} value={form.restart_delay} onChange={(v) => set("restart_delay", v)} />
                      <EditField label={t("apps.field.minUptime")} hint={t("apps.settings.minUptimeHint")} value={form.min_uptime} onChange={(v) => set("min_uptime", v)} />
                      <EditField label={t("apps.field.healthPath")} hint={t("apps.settings.healthHint")} value={form.health_path} onChange={(v) => set("health_path", v)} placeholder="/" />
                      <EditField label={t("apps.field.repo")} hint={t("apps.settings.repoHint")} value={form.repo} onChange={(v) => set("repo", v)} />
                      <EditField label={t("apps.field.ref")} hint={t("apps.settings.refHint")} value={form.ref} onChange={(v) => set("ref", v)} />
                      <label className="field-label md:col-span-2">
                        <span className="flex items-baseline justify-between gap-2">
                          <span>{t("apps.field.build")}</span>
                          <span className="normal-case tracking-normal text-muted-foreground/55">{t("apps.settings.buildHint")}</span>
                        </span>
                        <textarea
                          value={form.build}
                          onChange={(e) => setForm({ ...form, build: e.target.value })}
                          rows={4}
                          spellCheck={false}
                          placeholder={"pnpm install\npnpm build"}
                          className="w-full resize-y border border-input bg-transparent px-2 py-1.5 font-mono text-[11px] outline-none focus-visible:border-ring"
                        />
                      </label>
                      <label className="flex w-fit items-center gap-2 border border-border bg-card/30 px-2.5 py-1.5 text-[11px] text-muted-foreground md:col-span-2">
                        <Checkbox checked={form.auto} onCheckedChange={(v) => setForm({ ...form, auto: v === true })} />
                        {t("apps.settings.autoDeploy")}
                      </label>
                    </>
                  )}
                  <label className="flex w-fit items-center gap-2 border border-border bg-card/30 px-2.5 py-1.5 text-[11px] text-muted-foreground md:col-span-2">
                    <Checkbox checked={form.disabled} onCheckedChange={(v) => setForm({ ...form, disabled: v === true })} />
                    {t("apps.settings.disabled")}
                  </label>
                </div>
              </FormDialog>
              <FormDialog
                trigger={
                  <Button variant="destructive" size="sm">
                    <Trash2 className="h-3 w-3" /> {t("common.delete")}
                  </Button>
                }
                title={t("apps.settings.deleteTitle", { name })}
                description={t("apps.settings.deleteDesc")}
                submitLabel={t("apps.settings.deleteSubmit")}
                canSubmit={confirmDel === name}
                onSubmit={async () => {
                  const res = await deleteApp(name);
                  toast.success(t("apps.settings.deleted", { name, hint: res.hint ?? "" }));
                  navigate("/");
                }}
              >
                <Field label={t("apps.settings.confirmLabel")} hint={t("apps.settings.confirmHint", { name })}>
                  <Input
                    value={confirmDel}
                    onChange={(e) => setConfirmDel(e.target.value)}
                    placeholder={name}
                    className="h-8 border-bad/30 font-mono text-[12px]"
                    autoFocus
                  />
                </Field>
              </FormDialog>
            </div>
          ) : undefined
        }
      >
        <div className="grid grid-cols-1 gap-x-6 gap-y-4 md:grid-cols-3">
          <div>
            <p className="micro mb-1 text-signal">{t("apps.settings.groupNetwork")}</p>
            <Row label={t("apps.field.domain")} value={meta?.domains?.join(", ")} />
            <Row label={t("apps.field.port")} value={meta?.port} />
            <Row label={t("apps.field.memory")} value={meta?.memory_mb ? `${meta.memory_mb} MB` : t("apps.settings.noLimit")} />
          </div>
          <div>
            <p className="micro mb-1 text-signal">{t("apps.settings.groupProcess")}</p>
            <Row label={t("apps.field.command")} value={meta?.command} />
            <Row label={t("apps.field.args")} value={(meta?.args ?? []).join(" ")} />
            <Row label={t("apps.field.dir")} value={meta?.dir} />
            <Row label={t("apps.field.static")} value={meta?.static} />
            <Row label={t("apps.field.envFile")} value={meta?.env_file} />
            <Row label={t("apps.field.restart")} value={meta?.restart_delay} />
            <Row label={t("apps.field.minUptime")} value={meta?.min_uptime} />
            <Row label={t("apps.field.healthPath")} value={meta?.health_path || "/"} />
          </div>
          <div>
            <p className="micro mb-1 text-signal">{t("apps.settings.groupDeploy")}</p>
            {external ? (
              <Row label={t("apps.field.runtime")} value={t("apps.settings.externalRuntime")} />
            ) : (
              <>
                <Row label={t("apps.field.repo")} value={meta?.deploy?.repo} />
                <Row label={t("apps.field.ref")} value={meta?.deploy?.ref} />
                <Row label={t("apps.field.auto")} value={meta?.deploy?.auto ? t("common.yes") : t("common.no")} />
                <Row label={t("apps.field.build")} value={(meta?.deploy?.build ?? []).join(" · ")} />
              </>
            )}
          </div>
        </div>
      </Section>
      <p className="font-mono text-[9px] leading-relaxed text-muted-foreground/70">
        {t("apps.settings.secretsNote", { environment: t("nav.environment") })}
      </p>
    </div>
  );
}
