import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import { Pencil, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Section } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { PathInput } from "../../shared/components/PathPicker";
import { DomainSuggest } from "../../shared/components/DomainSuggest";
import { updateApp, deleteApp } from "./api";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

type TextKey = "command" | "args" | "dir" | "static" | "env_file" | "restart_delay" | "min_uptime" | "repo" | "ref";

// Ajustes por unidad: vista de solo lectura + diálogos de edición/borrado.
// Un solo guardado aplica el manifiesto; el supervisor reinicia lo cambiado.
export function AppSettings() {
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

  async function save() {
    if (external) {
      await updateApp(name, {
        dir: form.dir || undefined,
        env_file: form.env_file || "",
        port: form.port.trim() === "" ? undefined : Number(form.port),
        domain: form.domain.trim(),
        memory_mb: form.memory_mb.trim() === "" ? undefined : Number(form.memory_mb),
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
      domain: form.domain.trim(),
      restart_delay: form.restart_delay || undefined,
      min_uptime: form.min_uptime || undefined,
      memory_mb: form.memory_mb.trim() === "" ? undefined : Number(form.memory_mb),
      deploy: form.repo
        ? { repo: form.repo, ref: form.ref || "main", build: meta?.deploy?.build ?? [], auto: form.auto }
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
        title="Ajustes"
        description="vista de solo lectura · la edición vive en un diálogo"
        actions={
          canWrite ? (
            <div className="flex items-center gap-2">
              <FormDialog
                trigger={
                  <Button variant="outline" size="sm">
                    <Pencil className="h-3 w-3" /> editar
                  </Button>
                }
                title={`Editar ${name}`}
                description="guardar aplica el manifiesto · el supervisor reinicia lo que cambie"
                submitLabel="guardar"
                canSubmit={form.command.trim() !== ""}
                onSubmit={save}
                className="sm:max-w-2xl"
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
                        placeholder="app.localhost"
                        className="h-8 min-w-0 flex-1 font-mono text-[12px]"
                      />
                      {form.domain !== "" && (
                        <Button type="button" variant="destructive" size="xs" onClick={() => set("domain", "")}>
                          quitar
                        </Button>
                      )}
                    </div>
                    <DomainSuggest name={meta?.name ?? name} onPick={(h) => set("domain", h)} />
                  </div>
                  <EditField
                    label="puerto"
                    hint="local · retargeta la ruta"
                    value={form.port}
                    onChange={(v) => set("port", v.replace(/\D/g, ""))}
                    placeholder="3001"
                  />
                  <EditField
                    label="memoria"
                    hint="MB · 0 = sin tope"
                    value={form.memory_mb}
                    onChange={(v) => set("memory_mb", v.replace(/\D/g, ""))}
                    placeholder="sin tope"
                  />
                  <EditField label="comando" hint="binario absoluto" value={form.command} onChange={(v) => set("command", v)} pick="file" />
                  <EditField
                    label="argumentos"
                    hint="separados por espacios"
                    value={form.args}
                    onChange={(v) => set("args", v)}
                    placeholder="server.js"
                  />
                  <EditField label="directorio" hint="cwd de la app" value={form.dir} onChange={(v) => set("dir", v)} pick="dir" />
                  <EditField
                    label="estático"
                    hint="sirve archivos en vez de proceso"
                    value={form.static}
                    onChange={(v) => set("static", v)}
                    pick="dir"
                  />
                  <EditField label="env_file" hint="dotenv adicional" value={form.env_file} onChange={(v) => set("env_file", v)} pick="file" />
                  <EditField label="restart_delay" hint="espera entre reinicios" value={form.restart_delay} onChange={(v) => set("restart_delay", v)} />
                  <EditField label="min_uptime" hint="uptime mínimo sano" value={form.min_uptime} onChange={(v) => set("min_uptime", v)} />
                  <EditField label="repo" hint="git https o ruta local" value={form.repo} onChange={(v) => set("repo", v)} />
                  <EditField label="ref" hint="rama, tag o SHA" value={form.ref} onChange={(v) => set("ref", v)} />
                  <label className="flex w-fit items-center gap-2 border border-border bg-card/30 px-2.5 py-1.5 text-[11px] text-muted-foreground md:col-span-2">
                    <Checkbox checked={form.auto} onCheckedChange={(v) => setForm({ ...form, auto: v === true })} />
                    auto-deploy al recibir push
                  </label>
                </div>
              </FormDialog>
              <FormDialog
                trigger={
                  <Button variant="destructive" size="sm">
                    <Trash2 className="h-3 w-3" /> eliminar
                  </Button>
                }
                title={`Eliminar ${name}`}
                description="quita la app y sus rutas de la flota"
                submitLabel="eliminar app"
                canSubmit={confirmDel === name}
                onSubmit={async () => {
                  await deleteApp(name);
                  navigate("/deployments");
                }}
              >
                <Field label="confirmación" hint={`escribe ${name}`}>
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
            <p className="micro mb-1 text-signal">red</p>
            <Row label="dominio" value={meta?.domains?.join(", ")} />
            <Row label="puerto" value={meta?.port} />
            <Row label="memoria" value={meta?.memory_mb ? `${meta.memory_mb} MB` : "sin tope"} />
          </div>
          <div>
            <p className="micro mb-1 text-signal">proceso</p>
            <Row label="comando" value={meta?.command} />
            <Row label="args" value={(meta?.args ?? []).join(" ")} />
            <Row label="dir" value={meta?.dir} />
            <Row label="estático" value={meta?.static} />
            <Row label="env_file" value={meta?.env_file} />
            <Row label="restart" value={meta?.restart_delay} />
            <Row label="min_uptime" value={meta?.min_uptime} />
          </div>
          <div>
            <p className="micro mb-1 text-signal">despliegue</p>
            {external ? (
              <Row label="runtime" value="externo · solo proxy (sin build/deploy)" />
            ) : (
              <>
                <Row label="repo" value={meta?.deploy?.repo} />
                <Row label="ref" value={meta?.deploy?.ref} />
                <Row label="auto" value={meta?.deploy?.auto ? "sí" : "no"} />
                <Row label="build" value={(meta?.deploy?.build ?? []).join(" · ")} />
              </>
            )}
          </div>
        </div>
      </Section>
      <p className="font-mono text-[9px] leading-relaxed text-muted-foreground/70">
        Los secretos no se editan aquí: viven en Entorno (globales y por proyecto).
      </p>
    </div>
  );
}
