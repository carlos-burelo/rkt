import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { PathInput } from "../../shared/components/PathPicker";
import { DomainSuggest } from "../../shared/components/DomainSuggest";
import { Field, ORIGINS, type Origin } from "./createDialog.model";
import type { DetectOut, GithubRepo } from "./api";

export interface StepErrors {
  repo: string;
  name: string;
  dir: string;
  command: string;
  gitRepo: string;
}

// Paso 1: origen de la unidad.
export function OriginStep({
  origin,
  setOrigin,
}: {
  origin: Origin;
  setOrigin: (o: Origin) => void;
}) {
  return (
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
            <span
              className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center border ${
                on ? "border-signal/60 text-signal" : "border-border text-muted-foreground"
              }`}
            >
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
  );
}

// Paso 2: detalles editables.
export function DetailsStep({
  origin,
  repo,
  setRepo,
  ghRepos,
  repoPick,
  toggleRepos,
  setRepoPick,
  repoQ,
  setRepoQ,
  reposFiltered,
  name,
  setName,
  setNameTouched,
  domain,
  setDomain,
  command,
  setCommand,
  dir,
  setDir,
  auto,
  setAuto,
  advanced,
  setAdvanced,
  port,
  setPort,
  errs,
  tried,
  next,
}: {
  origin: Origin;
  repo: string;
  setRepo: (v: string) => void;
  ghRepos: GithubRepo[] | null;
  repoPick: boolean;
  toggleRepos: () => void;
  setRepoPick: (v: boolean) => void;
  repoQ: string;
  setRepoQ: (v: string) => void;
  reposFiltered: GithubRepo[];
  name: string;
  setName: (v: string) => void;
  setNameTouched: (v: boolean) => void;
  domain: string;
  setDomain: (v: string) => void;
  command: string;
  setCommand: (v: string) => void;
  dir: string;
  setDir: (v: string) => void;
  auto: boolean;
  setAuto: (v: boolean) => void;
  advanced: boolean;
  setAdvanced: (v: boolean) => void;
  port: string;
  setPort: (v: string) => void;
  errs: StepErrors;
  tried: boolean;
  next: () => void;
}) {
  return (
    <div className="space-y-3">
      {origin === "git" && (
        <Field label="repositorio" hint="https:// o git@" error={tried ? errs.gitRepo || errs.repo : ""}>
          <div className="flex items-center gap-1">
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
            <Button type="button" size="sm" variant="outline" onClick={toggleRepos}>
              GitHub
            </Button>
          </div>
          <span className="font-mono text-[9px] normal-case text-muted-foreground/70">
            se clona en segundo plano; el comando se detecta del proyecto
          </span>
          {repoPick && (
            <div className="mt-1 border border-border bg-card/40">
              <div className="border-b border-border p-1.5">
                <Input
                  value={repoQ}
                  onChange={(e) => setRepoQ(e.target.value)}
                  placeholder="buscar repo u org…"
                  className="h-7 font-mono text-[10px]"
                  autoFocus
                />
              </div>
              <div className="max-h-52 overflow-auto">
                {ghRepos === null && (
                  <p className="px-2 py-3 font-mono text-[9px] text-muted-foreground">cargando repositorios de la App…</p>
                )}
                {ghRepos !== null && reposFiltered.length === 0 && (
                  <p className="px-2 py-3 font-mono text-[9px] text-muted-foreground">
                    sin repos visibles · instala la App con All repositories
                  </p>
                )}
                {reposFiltered.map((r) => (
                  <button
                    key={r.full_name}
                    type="button"
                    onClick={() => {
                      setRepo(`https://github.com/${r.full_name}.git`);
                      setRepoPick(false);
                    }}
                    className="flex w-full items-center gap-2 border-b border-border/50 px-2 py-1.5 text-left last:border-0 hover:bg-accent/40"
                  >
                    <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{r.full_name}</span>
                    <span className="shrink-0 font-mono text-[8px] text-muted-foreground">{r.default_branch}</span>
                    {r.private && (
                      <span className="shrink-0 border border-warn/40 px-1 font-mono text-[8px] text-warn">privado</span>
                    )}
                  </button>
                ))}
              </div>
            </div>
          )}
        </Field>
      )}
      {origin === "local" && (
        <Field label="directorio" hint="ruta en esta máquina" error={tried ? errs.dir : ""}>
          <PathInput
            value={dir}
            onChange={setDir}
            mode="dir"
            placeholder="C:\proyectos\mi-app"
            ariaLabel="directorio"
          />
          <span className="font-mono text-[9px] normal-case text-muted-foreground/70">
            el comando se infiere del proyecto; si falla, define comando o configura node_bin/python_bin en Ajustes
          </span>
        </Field>
      )}
      {origin === "external" && (
        <Field label="carpeta del repo" hint="para verla y sugerir el comando" error={tried ? errs.dir : ""}>
          <PathInput
            value={dir}
            onChange={setDir}
            mode="dir"
            placeholder="C:\Users\x\dev\mi-app"
            ariaLabel="carpeta del repo"
          />
          <span className="font-mono text-[9px] normal-case text-muted-foreground/70">
            rkt NO arranca nada: solo enruta el dominio al puerto. El dev server corre en tu terminal.
          </span>
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

      <DomainSuggest name={name} onPick={setDomain} />

      {origin === "manual" && (
        <Field label="comando" hint="binario absoluto" error={tried ? errs.command : ""}>
          <PathInput
            value={command}
            onChange={setCommand}
            mode="file"
            placeholder="C:\apps\mi-app\mi-app.exe"
            ariaLabel="comando"
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
        {advanced ? "ocultar avanzado" : "avanzado · comando, directorio y puerto"}
      </button>

      {advanced && (
        <div className="grid grid-cols-1 gap-3 border border-border bg-card/40 p-3 sm:grid-cols-2">
          {origin !== "external" && (
            <Field label="comando" hint="anula la detección">
              <PathInput
                value={command}
                onChange={setCommand}
                mode="file"
                placeholder="C:\apps\mi-app\mi-app.exe"
                ariaLabel="comando avanzado"
              />
            </Field>
          )}
          <Field label="directorio" hint="por defecto C:\apps\<nombre>">
            <PathInput
              value={dir}
              onChange={setDir}
              mode="dir"
              placeholder="C:\apps\mi-app"
              ariaLabel="directorio avanzado"
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
        </div>
      )}
    </div>
  );
}

// Paso 3: revisión y opción de compilar/desplegar al crear.
export function ReviewStep({
  origin,
  name,
  repo,
  dir,
  command,
  domain,
  auto,
  port,
  plan,
  planBusy,
  buildNow,
  setBuildNow,
}: {
  origin: Origin;
  name: string;
  repo: string;
  dir: string;
  command: string;
  domain: string;
  auto: boolean;
  port: string;
  plan: DetectOut | null;
  planBusy: boolean;
  buildNow: boolean;
  setBuildNow: (v: boolean) => void;
}) {
  return (
    <div className="space-y-3">
      {origin === "local" && (
        <div className="border border-border bg-card/40">
          <div className="flex flex-wrap items-center gap-2 border-b border-border bg-secondary/20 px-3 py-1.5">
            <span aria-hidden className="h-1.5 w-1.5 bg-signal" />
            <span className="micro">detección</span>
            <span className="flex-1" />
            {planBusy ? (
              <span className="font-mono text-[9px] text-muted-foreground">analizando…</span>
            ) : plan ? (
              <span className="border border-signal/40 bg-signal/10 px-1.5 font-mono text-[9px] tracking-[0.08em] uppercase text-signal">
                {plan.framework}
                {plan.pm ? ` · ${plan.pm}` : ""}
              </span>
            ) : (
              <span className="font-mono text-[9px] text-warn">sin plan detectado</span>
            )}
          </div>
          <div className="space-y-1.5 px-3 py-2">
            {plan && (
              <>
                <p className="font-mono text-[10px] text-muted-foreground">
                  instala: {plan.install.length ? plan.install.join(" · ") : "nada"}
                </p>
                <p className="font-mono text-[10px] text-muted-foreground">
                  compila: {plan.build.length ? plan.build.join(" · ") : "nada"}
                </p>
                <p className="truncate font-mono text-[10px] text-muted-foreground">
                  {plan.static_dir
                    ? `sirve estático: ${plan.static_abs}`
                    : `arranca: ${plan.start.join(" ") || "—"}`}
                </p>
              </>
            )}
            {!plan && !planBusy && (
              <p className="font-mono text-[10px] leading-relaxed text-muted-foreground">
                no reconocí el proyecto; podrás definir comando en avanzado o en Ajustes.
              </p>
            )}
          </div>
        </div>
      )}

      {origin === "git" && (
        <p className="font-mono text-[10px] leading-relaxed text-muted-foreground">
          el plan se detecta al clonar; el despliegue muestra cada paso.
        </p>
      )}

      {origin === "external" && (
        <p className="border border-border bg-card/40 px-3 py-2 font-mono text-[10px] leading-relaxed text-muted-foreground">
          app externa: sin build, sin deploy, sin supervisor. rkt solo enruta el dominio al puerto.
          Levantá el dev server en la carpeta con el puerto indicado.
        </p>
      )}

      <div className="border border-border">
        {[
          ["origen", ORIGINS.find((o) => o.id === origin)?.label ?? origin],
          ["nombre", name],
          ...(repo.trim() ? [["repositorio", repo.trim()]] : []),
          ...(dir.trim() ? [["directorio", dir.trim()]] : []),
          ["comando", origin === "external" ? "lo corrés vos (dev server externo)" : command.trim() || (plan?.start.join(" ") ?? "lo detecta rkt")],
          ["dominio", domain.trim() || "sin dominio (puedes asignarlo después)"],
          ...(origin === "git" ? [["auto-deploy", auto ? "sí, en cada push a main" : "no"]] : []),
          ...(origin === "external" && port.trim() ? [["puerto", port.trim()]] : []),
        ].map(([k, v]) => (
          <div key={k} className="flex items-baseline gap-3 border-b border-border/60 px-3 py-2 last:border-0">
            <span className="micro w-28 shrink-0">{k}</span>
            <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{v}</span>
          </div>
        ))}
      </div>

      {(origin === "git" || origin === "local") && (
        <label className="flex items-center gap-2 font-mono text-[10px] text-muted-foreground">
          <Checkbox checked={buildNow} onCheckedChange={(v) => setBuildNow(v === true)} />
          {origin === "git" ? "desplegar al crear" : "compilar al crear"}
        </label>
      )}
    </div>
  );
}
