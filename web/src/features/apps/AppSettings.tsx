import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Section } from "../../shared/components/patterns";
import { PathInput } from "../../shared/components/PathPicker";
import { DomainSuggest } from "../../shared/components/DomainSuggest";
import { updateApp, deleteApp } from "./api";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

type TextKey = "command" | "args" | "dir" | "static" | "env_file" | "restart_delay" | "min_uptime" | "repo" | "ref";

// Ajustes por unidad: red (dominio/puerto), proceso y deploy.
// Un solo guardado aplica el manifiesto; el supervisor reinicia lo cambiado.
export function AppSettings() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const { metas } = useFleet();
  const fail = useUI((u) => u.fail);
  const { canWrite } = useAuth();
  const meta = metas.find((m) => m.name === name);
  const [form, setForm] = useState({
    command: "",
    args: "",
    dir: "",
    static: "",
    env_file: "",
    port: "",
    domain: "",
    memory_mb: "",
    restart_delay: "",
    min_uptime: "",
    repo: "",
    ref: "",
    auto: false,
  });
  const [confirmDel, setConfirmDel] = useState("");

  useEffect(() => {
    setForm({
      command: meta?.command ?? "",
      args: (meta?.args ?? []).join(" "),
      dir: meta?.dir ?? "",
      static: meta?.static ?? "",
      env_file: meta?.env_file ?? "",
      port: meta?.port ?? "",
      domain: meta?.domains?.[0] ?? "",
      memory_mb: meta?.memory_mb ? String(meta.memory_mb) : "",
      restart_delay: meta?.restart_delay ?? "",
      min_uptime: meta?.min_uptime ?? "",
      repo: meta?.deploy?.repo ?? "",
      ref: meta?.deploy?.ref ?? "",
      auto: meta?.deploy?.auto ?? false,
    });
  }, [meta]);

  const set = (k: TextKey | "port" | "domain" | "memory_mb", v: string) => setForm((f) => ({ ...f, [k]: v }));

  function save() {
    updateApp(name, {
      command: form.command || undefined,
      args: form.args.trim() === "" ? [] : form.args.trim().split(/\s+/),
      dir: form.dir || undefined,
      env_file: form.env_file || "",
      static: form.static || "",
      port: form.port.trim() === "" ? undefined : Number(form.port),
      domain: form.domain.trim(),
      restart_delay: form.restart_delay || undefined,
      min_uptime: form.min_uptime || undefined,
      memory_mb: form.memory_mb.trim() === "" ? undefined : Number(form.memory_mb),
      deploy: form.repo
        ? { repo: form.repo, ref: form.ref || "main", build: meta?.deploy?.build ?? [], auto: form.auto }
        : null,
    }).catch(fail);
  }

  function Field({
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
      <label className="field-label">
        <span className="flex items-baseline justify-between gap-2">
          <span>{label}</span>
          <span className="normal-case tracking-normal text-muted-foreground/55">{hint}</span>
        </span>
        {pick ? (
          <PathInput
            value={value}
            onChange={onChange}
            mode={pick}
            disabled={!canWrite}
            ariaLabel={label}
            className="h-7 min-w-0 flex-1 font-mono text-[11px]"
          />
        ) : (
          <Input
            value={value}
            onChange={(e) => onChange(e.target.value)}
            disabled={!canWrite}
            placeholder={placeholder}
            className="h-7 font-mono text-[11px]"
          />
        )}
      </label>
    );
  }

  return (
    <div className="space-y-4">
      <Section
        title="Red"
        description="dominio y puerto · guardar reescribe la ruta del proxy"
        actions={
          canWrite ? (
            <Button variant="outline" size="sm" onClick={save}>
              Guardar
            </Button>
          ) : undefined
        }
      >
        <div className="grid grid-cols-1 gap-x-3 gap-y-2.5 md:grid-cols-2">
          <div className="field-label md:col-span-2">
            <span className="flex items-baseline justify-between gap-2">
              <span>dominio</span>
              <span className="normal-case tracking-normal text-muted-foreground/55">
                vacío = sin dominio · se retira la ruta anterior
              </span>
            </span>
            <div className="flex items-center gap-2">
              <Input
                value={form.domain}
                onChange={(e) => set("domain", e.target.value)}
                disabled={!canWrite}
                placeholder="app.localhost"
                className="h-7 min-w-0 flex-1 font-mono text-[11px]"
              />
              {form.domain !== "" && canWrite && (
                <Button variant="ghost" size="xs" onClick={() => set("domain", "")}>
                  quitar
                </Button>
              )}
            </div>
            <DomainSuggest name={meta?.name ?? name} onPick={(h) => set("domain", h)} />
          </div>
          <Field label="puerto" hint="local · retargeta la ruta" value={form.port} onChange={(v) => set("port", v.replace(/\D/g, ""))} placeholder="3001" />
          <Field label="memoria" hint="MB · 0 = sin tope" value={form.memory_mb} onChange={(v) => set("memory_mb", v.replace(/\D/g, ""))} placeholder="sin tope" />
        </div>
      </Section>

      <Section title="Proceso" description="cómo se ejecuta la unidad · aplicar reinicia la app">
        <div className="grid grid-cols-1 gap-x-3 gap-y-2.5 md:grid-cols-2">
          <Field label="comando" hint="binario absoluto" value={form.command} onChange={(v) => set("command", v)} pick="file" />
          <Field label="argumentos" hint="separados por espacios" value={form.args} onChange={(v) => set("args", v)} placeholder="server.js --port 3001" />
          <Field label="directorio" hint="cwd de la app" value={form.dir} onChange={(v) => set("dir", v)} pick="dir" />
          <Field label="estático" hint="si sirve archivos en vez de proceso" value={form.static} onChange={(v) => set("static", v)} pick="dir" />
          <Field label="env_file" hint="dotenv adicional" value={form.env_file} onChange={(v) => set("env_file", v)} pick="file" />
          <Field label="restart_delay" hint="espera entre reinicios" value={form.restart_delay} onChange={(v) => set("restart_delay", v)} />
          <Field label="min_uptime" hint="uptime mínimo sano" value={form.min_uptime} onChange={(v) => set("min_uptime", v)} />
        </div>
      </Section>

      <Section title="Despliegue" description="repo git y build · el plan detectado manda si no defines build">
        <div className="grid grid-cols-1 gap-x-3 gap-y-2.5 md:grid-cols-2">
          <Field label="repo" hint="git https o ruta local" value={form.repo} onChange={(v) => set("repo", v)} />
          <Field label="ref" hint="rama, tag o SHA" value={form.ref} onChange={(v) => set("ref", v)} />
        </div>
        {canWrite && (
          <label className="mt-3 flex w-fit items-center gap-2 border border-border bg-card/30 px-2.5 py-1.5 text-[11px] text-muted-foreground">
            <Checkbox checked={form.auto} onCheckedChange={(v) => setForm({ ...form, auto: v === true })} />
            auto-deploy al recibir push
          </label>
        )}
      </Section>

      {canWrite && (
        <section className="border border-bad/30 bg-bad/4">
          <header className="flex flex-wrap items-center gap-2 border-b border-bad/25 bg-bad/8 px-3 py-2">
            <span aria-hidden className="h-1.5 w-1.5 bg-bad" />
            <h2 className="text-[12px] font-medium text-bad">Zona de riesgo</h2>
            <p className="hidden text-[11px] text-bad/65 sm:block">
              borrar quita la app de la flota; las routes quedan huérfanas
            </p>
          </header>
          <div className="flex flex-col gap-2 p-3 sm:flex-row">
            <Input
              value={confirmDel}
              onChange={(e) => setConfirmDel(e.target.value)}
              placeholder={`escribe ${name} para borrar`}
              className="h-7 flex-1 border-bad/30 font-mono text-[11px]"
            />
            <Button
              variant="destructive"
              size="sm"
              disabled={confirmDel !== name}
              onClick={() => {
                deleteApp(name).then(() => navigate("/deployments")).catch(fail);
              }}
            >
              borrar app
            </Button>
          </div>
        </section>
      )}
    </div>
  );
}
