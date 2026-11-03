import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Section } from "../../shared/components/patterns";
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

  return (
    <div className="space-y-4">
      <Section
        title="Settings"
        description="Guardar valida y aplica (restart requerido para runtime)."
        actions={
          canWrite ? (
            <Button
              variant="outline" size="sm"
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
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          {(
            [
              ["command", form.command, (v: string) => setForm({ ...form, command: v })],
              ["dir", form.dir, (v: string) => setForm({ ...form, dir: v })],
              ["restart_delay", form.restart_delay, (v: string) => setForm({ ...form, restart_delay: v })],
              ["min_uptime", form.min_uptime, (v: string) => setForm({ ...form, min_uptime: v })],
              ["repo", form.repo, (v: string) => setForm({ ...form, repo: v })],
              ["ref", form.ref, (v: string) => setForm({ ...form, ref: v })],
            ] as const
          ).map(([label, v, set]) => (
            <label key={label} className="block">
              <span className="font-mono text-[11px] text-muted-foreground">{label}</span>
              <Input value={v} onChange={(e) => set(e.target.value)} disabled={!canWrite} className="mt-1 font-mono text-xs" />
            </label>
          ))}
          {canWrite && (
            <label className="flex items-center gap-2 text-xs text-muted-foreground">
              <Checkbox checked={form.auto} onCheckedChange={(v) => setForm({ ...form, auto: v === true })} />
              auto-deploy
            </label>
          )}
        </div>
      </Section>

      {canWrite && (
        <Section title="Danger zone" description="Borrar quita la app de la flota. Las routes quedan huérfanas: revísalas en Domains.">
          <div className="flex gap-2">
            <Input value={confirmDel} onChange={(e) => setConfirmDel(e.target.value)}
              placeholder={`escribe ${name} para borrar`}
              className="flex-1 border-destructive/50 font-mono text-xs" />
            <Button
              variant="destructive" size="sm"
              disabled={confirmDel !== name}
              onClick={() => {
                deleteApp(name).then(() => navigate("/deployments")).catch(fail);
              }}
            >
              borrar app
            </Button>
          </div>
        </Section>
      )}
    </div>
  );
}
