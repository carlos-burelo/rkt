import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, ArrowRight, Check, FolderGit2, FolderOpen, Terminal } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { createApp, CreateAppIn } from "./api";

type Origin = "git" | "local" | "manual";
type Step = 1 | 2 | 3;

const ORIGINS: { id: Origin; label: string; hint: string; icon: typeof FolderGit2 }[] = [
  { id: "git", label: "Repositorio Git", hint: "Clona el repo y detecta comando, puerto y dominio.", icon: FolderGit2 },
  { id: "local", label: "Directorio local", hint: "Sirve un proyecto que ya vive en esta máquina.", icon: FolderOpen },
  { id: "manual", label: "Manual", hint: "Sin repo: defines el binario y el directorio a mano.", icon: Terminal },
];

const SLUG = /^[a-z0-9][a-z0-9-]*$/;

// slug desde la última parte del repo (git o ruta local).
function slugOf(src: string): string {
  const clean = src.trim().replace(/[\\/]+$/, "").replace(/\.git$/i, "");
  const base = clean.split(/[\\/]/).pop() ?? "";
  return base
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40);
}

function looksGit(src: string): boolean {
  return /^(https?:\/\/|git@|ssh:\/\/)/i.test(src.trim());
}

function Field({
  label,
  hint,
  error,
  children,
}: {
  label: string;
  hint?: string;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <label className="field-label">
      <span className="flex items-baseline justify-between gap-2">
        {label}
        {hint && <span className="font-mono text-[9px] text-muted-foreground/70 normal-case">{hint}</span>}
      </span>
      {children}
      {error && <span className="font-mono text-[10px] normal-case text-bad">{error}</span>}
    </label>
  );
}

// Alta guiada por pasos: origen → detalles → revisión. Una sola acción
// primaria por paso; el backend infiere lo que falta (cfgfile.CreateApp).
export function CreateDialog({
  open,
  setOpen,
  onCreated,
  goDomains,
  fail,
}: {
  open: boolean;
  setOpen: (b: boolean) => void;
  onCreated: (name: string) => void;
  goDomains: () => void;
  fail: (e: unknown) => void;
}) {
  const [step, setStep] = useState<Step>(1);
  const [origin, setOrigin] = useState<Origin>("git");
  const [repo, setRepo] = useState("");
  const [name, setName] = useState("");
  const [nameTouched, setNameTouched] = useState(false);
  const [command, setCommand] = useState("");
  const [dir, setDir] = useState("");
  const [domain, setDomain] = useState("");
  const [port, setPort] = useState("");
  const [preset, setPreset] = useState("");
  const [auto, setAuto] = useState(true);
  const [advanced, setAdvanced] = useState(false);
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState(false);

  // nombre derivado mientras el usuario no lo escriba
  const suggested = useMemo(() => slugOf(origin === "git" ? repo : dir), [origin, repo, dir]);
  useEffect(() => {
    if (!nameTouched) setName(suggested);
  }, [suggested, nameTouched]);

  function reset() {
    setStep(1);
    setOrigin("git");
    setRepo("");
    setName("");
    setNameTouched(false);
    setCommand("");
    setDir("");
    setDomain("");
    setPort("");
    setPreset("");
    setAuto(true);
    setAdvanced(false);
    setTried(false);
    setBusy(false);
  }

  const errs = {
    repo: origin === "git" && repo.trim() !== "" && !looksGit(repo) ? "usa https:// o git@…" : "",
    name: !SLUG.test(name) ? "minúsculas, números y guiones" : "",
    dir: origin === "local" && dir.trim() === "" ? "ruta absoluta del proyecto" : "",
    command: origin === "manual" && command.trim() === "" ? "binario absoluto" : "",
    gitRepo: origin === "git" && repo.trim() === "" ? "pega la URL del repositorio" : "",
  };
  const step2ok = !errs.repo && !errs.name && !errs.dir && !errs.command && !errs.gitRepo;

  function next() {
    if (step === 2) {
      setTried(true);
      if (!step2ok) return;
      setTried(false);
    }
    setStep((s) => (s < 3 ? ((s + 1) as Step) : s));
  }

  function submit() {
    const input: CreateAppIn = {
      repo: origin === "git" ? repo.trim() : undefined,
      name: name.trim() || undefined,
      command: command.trim() || undefined,
      dir: dir.trim() || undefined,
      domain: domain.trim() || undefined,
      port: port.trim() === "" ? undefined : Number(port),
      preset: preset.trim() || undefined,
      auto: origin === "git" ? auto : false,
    };
    setBusy(true);
    createApp(input)
      .then((app) => {
        setBusy(false);
        setOpen(false);
        reset();
        onCreated(app);
      })
      .catch((e) => {
        setBusy(false);
        fail(e);
      });
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o: boolean) => {
        if (!o) {
          setOpen(false);
          reset();
        }
      }}
    >
      <DialogContent className="gap-4 sm:max-w-lg">
        <DialogHeader>
          <div className="flex items-center gap-2 pr-8">
            <span aria-hidden className="h-1 w-4 bg-signal" />
            <span className="micro">alta de unidad</span>
            <span className="flex-1" />
            <span className="tnum font-mono text-[9px] text-muted-foreground">paso {step} de 3</span>
          </div>
          <DialogTitle className="mt-1 text-[18px] tracking-[-0.02em]">
            {step === 1 ? "¿De dónde sale la app?" : step === 2 ? "Detalles de la unidad" : "Revisa y crea"}
          </DialogTitle>
          <DialogDescription className="font-mono text-[10px] leading-relaxed">
            {step === 1
              ? "rkt infiere comando, puerto y dominio cuando el origen lo permite."
              : step === 2
                ? "solo el nombre es obligatorio; el resto lo completa el descubrimiento."
                : "esto escribe el manifiesto y recarga el proxy. Nada se despliega hasta que lo pidas."}
          </DialogDescription>
        </DialogHeader>

        {step === 1 && (
          <div className="space-y-1.5">
            {ORIGINS.map((o) => {
              const on = origin === o.id;
              const Icon = o.icon;
              return (
                <button
                  key={o.id}
                  type="button"
                  onClick={() => setOrigin(o.id)}
                  aria-pressed={on}
                  className={`flex w-full items-start gap-3 border px-3 py-2.5 text-left transition-colors ${
                    on ? "border-signal/70 bg-secondary/40" : "border-border hover:border-input hover:bg-accent/40"
                  }`}
                >
                  <span className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center border ${on ? "border-signal/60 text-signal" : "border-border text-muted-foreground"}`}>
                    <Icon className="h-3.5 w-3.5" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-[12px] font-medium">{o.label}</span>
                    <span className="mt-0.5 block font-mono text-[10px] leading-relaxed text-muted-foreground">{o.hint}</span>
                  </span>
                  <span className={`mt-1 h-1.5 w-1.5 shrink-0 ${on ? "bg-signal" : "bg-border"}`} aria-hidden />
                </button>
              );
            })}
          </div>
        )}

        {step === 2 && (
          <div className="space-y-3">
            {origin === "git" && (
              <Field label="repositorio" hint="https:// o git@" error={tried ? errs.gitRepo || errs.repo : ""}>
                <Input
                  autoFocus
                  value={repo}
                  onChange={(e) => setRepo(e.target.value)}
                  placeholder="https://github.com/usuario/app"
                  className="font-mono text-[11px]"
                  onKeyDown={(e) => {
                    if (e.key === "Enter") next();
                  }}
                />
                <span className="font-mono text-[9px] normal-case text-muted-foreground/70">
                  se clona en segundo plano; el comando se detecta del proyecto
                </span>
              </Field>
            )}
            {origin === "local" && (
              <Field label="directorio" hint="ruta en esta máquina" error={tried ? errs.dir : ""}>
                <Input
                  autoFocus
                  value={dir}
                  onChange={(e) => setDir(e.target.value)}
                  placeholder="C:\proyectos\mi-app"
                  className="font-mono text-[11px]"
                />
              </Field>
            )}

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <Field label="nombre" hint="[a-z0-9-]" error={tried ? errs.name : ""}>
                <Input
                  value={name}
                  onChange={(e) => {
                    setNameTouched(true);
                    setName(e.target.value);
                  }}
                  placeholder="mi-app"
                  className="font-mono text-[11px]"
                />
              </Field>
              <Field label="dominio" hint="opcional">
                <Input
                  value={domain}
                  onChange={(e) => setDomain(e.target.value)}
                  placeholder="app.midominio.com"
                  className="font-mono text-[11px]"
                />
              </Field>
            </div>

            {origin === "manual" && (
              <Field label="comando" hint="binario absoluto" error={tried ? errs.command : ""}>
                <Input
                  autoFocus
                  value={command}
                  onChange={(e) => setCommand(e.target.value)}
                  placeholder="C:\apps\mi-app\mi-app.exe"
                  className="font-mono text-[11px]"
                />
              </Field>
            )}

            {origin === "git" && (
              <label className="flex items-center gap-2 font-mono text-[10px] text-muted-foreground">
                <Checkbox checked={auto} onCheckedChange={(v) => setAuto(v === true)} />
                desplegar en cada push a main
              </label>
            )}

            <button
              type="button"
              onClick={() => setAdvanced(!advanced)}
              className="font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:text-foreground"
              aria-expanded={advanced}
            >
              {advanced ? "ocultar avanzado" : "avanzado · comando, puerto, preset"}
            </button>

            {advanced && (
              <div className="grid grid-cols-1 gap-3 border border-border bg-card/40 p-3 sm:grid-cols-2">
                <Field label="comando" hint="anula la detección">
                  <Input
                    value={command}
                    onChange={(e) => setCommand(e.target.value)}
                    placeholder="C:\apps\mi-app\mi-app.exe"
                    className="font-mono text-[11px]"
                  />
                </Field>
                <Field label="directorio" hint="por defecto C:\apps\<nombre>">
                  <Input
                    value={dir}
                    onChange={(e) => setDir(e.target.value)}
                    placeholder="C:\apps\mi-app"
                    className="font-mono text-[11px]"
                  />
                </Field>
                <Field label="puerto" hint="opcional">
                  <Input
                    value={port}
                    onChange={(e) => setPort(e.target.value.replace(/\D/g, ""))}
                    inputMode="numeric"
                    placeholder="3000"
                    className="tnum font-mono text-[11px]"
                  />
                </Field>
                <Field label="preset" hint="si la detección falla">
                  <Input
                    value={preset}
                    onChange={(e) => setPreset(e.target.value)}
                    placeholder="node, go, python…"
                    className="font-mono text-[11px]"
                  />
                </Field>
              </div>
            )}
          </div>
        )}

        {step === 3 && (
          <div className="border border-border">
            {[
              ["origen", ORIGINS.find((o) => o.id === origin)?.label ?? origin],
              ["nombre", name],
              ...(repo.trim() ? [["repositorio", repo.trim()]] : []),
              ...(dir.trim() ? [["directorio", dir.trim()]] : []),
              ["comando", command.trim() || "lo detecta rkt"],
              ["dominio", domain.trim() || "sin dominio (puedes asignarlo después)"],
              ...(origin === "git" ? [["auto-deploy", auto ? "sí, en cada push a main" : "no"] ] : []),
            ].map(([k, v]) => (
              <div key={k} className="flex items-baseline gap-3 border-b border-border/60 px-3 py-2 last:border-0">
                <span className="micro w-28 shrink-0">{k}</span>
                <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{v}</span>
              </div>
            ))}
          </div>
        )}

        <div className="flex flex-wrap items-center gap-2 border-t border-border pt-3">
          {step > 1 && (
            <Button variant="ghost" size="sm" onClick={() => setStep((s) => (s - 1) as Step)} disabled={busy}>
              <ArrowLeft className="h-3 w-3" /> atrás
            </Button>
          )}
          {step === 1 && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setOpen(false);
                goDomains();
              }}
            >
              ir a Dominios
            </Button>
          )}
          <span className="flex-1" />
          {step < 3 ? (
            <Button size="sm" onClick={next}>
              continuar <ArrowRight className="h-3 w-3" />
            </Button>
          ) : (
            <Button size="sm" onClick={submit} disabled={busy || !name.trim()}>
              <Check className="h-3 w-3" /> {busy ? "creando…" : "crear aplicación"}
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
