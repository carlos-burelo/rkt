import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Section } from "../../shared/components/patterns";
import { PathInput } from "../../shared/components/PathPicker";
import { updateApp, deleteApp } from "./api";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

export function AppSettings() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const { metas } = useFleet();
  const fail = useUI((u) => u.fail);
  const { canWrite } = useAuth();
  const meta = metas.find((m) => m.name === name);
  const [form, setForm] = useState({ command: "", dir: "", restart_delay: "", min_uptime: "", repo: "", ref: "", auto: false });
  const [confirmDel, setConfirmDel] = useState("");

  useEffect(() => {
    setForm({
      command: meta?.command ?? "",
      dir: meta?.dir ?? "",
      restart_delay: meta?.restart_delay ?? "",
      min_uptime: meta?.min_uptime ?? "",
      repo: meta?.deploy?.repo ?? "",
      ref: meta?.deploy?.ref ?? "",
      auto: meta?.deploy?.auto ?? false,
    });
  }, [meta]);

type StringField = "command" | "dir" | "restart_delay" | "min_uptime" | "repo" | "ref";

  const fields: { label: string; key: StringField; hint: string; pick?: "dir" | "file" }[] = [
    { label: "command", key: "command", hint: "binario o comando de arranque", pick: "file" },
    { label: "dir", key: "dir", hint: "directorio de trabajo", pick: "dir" },
    { label: "restart_delay", key: "restart_delay", hint: "espera entre reinicios" },
    { label: "min_uptime", key: "min_uptime", hint: "uptime mínimo para contar sano" },
    { label: "repo", key: "repo", hint: "git https o ruta local" },
    { label: "ref", key: "ref", hint: "rama o tag" },
  ];

  return (
    <div className="space-y-4">
      <Section
        title="Configuración de unidad"
        description="guardar valida y aplica · runtime exige restart"
        actions={
          canWrite ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                updateApp(name, {
                  command: form.command || undefined,
                  dir: form.dir || undefined,
                  restart_delay: form.restart_delay || undefined,
                  min_uptime: form.min_uptime || undefined,
                  deploy: form.repo
                    ? { repo: form.repo, ref: form.ref || "main", build: meta?.deploy?.build ?? [], auto: form.auto }
                    : null,
                }).catch(fail);
              }}
            >
              Guardar
            </Button>
          ) : undefined
        }
      >
        <div className="grid grid-cols-1 gap-x-3 gap-y-2.5 md:grid-cols-2">
          {fields.map((f) => (
            <label key={f.key} className="field-label">
              <span className="flex items-baseline justify-between gap-2">
                <span>{f.label}</span>
                <span className="normal-case tracking-normal text-muted-foreground/55">{f.hint}</span>
              </span>
              {f.pick ? (
                <PathInput
                  value={form[f.key]}
                  onChange={(v) => setForm({ ...form, [f.key]: v })}
                  mode={f.pick}
                  disabled={!canWrite}
                  ariaLabel={f.label}
                  className="h-7 min-w-0 flex-1 font-mono text-[11px]"
                />
              ) : (
                <Input
                  value={form[f.key]}
                  onChange={(e) => setForm({ ...form, [f.key]: e.target.value })}
                  disabled={!canWrite}
                  className="h-7 font-mono text-[11px]"
                />
              )}
            </label>
          ))}
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
            <h2 className="text-[12px] font-medium text-bad">Danger zone</h2>
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
