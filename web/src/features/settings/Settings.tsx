import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";
import { getSettings, setSetting, getGithubStatus, GithubStatus } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader, Section } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { PathInput } from "../../shared/components/PathPicker";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

const DESCRIPTIONS: Record<string, string> = {
  node_bin: "Binario node para presets next/node.",
  python_bin: "Binario python para preset python.",
  base_domain: "Dominio base para sugerir <app>.<base>.",
  port_pool: "Rango de puertos locales (ej 3000-3999).",
  site_name: "Nombre visible en rail y cabecera.",
  quota_reqs: "Tope de peticiones 30d para el medidor (0 = sin tope).",
  quota_bytes: "Tope de tráfico 30d para el medidor (0 = sin tope).",
};

const PICKERS: Record<string, "dir" | "file"> = {
  node_bin: "file",
  python_bin: "file",
};

export function Settings() {
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const [settings, setSettings] = useState<Record<string, string>>({});
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [gh, setGh] = useState<GithubStatus | null>(null);
  const [params, setParams] = useSearchParams();
  const justInstalled = params.get("github") === "ok";

  async function load() {
    try {
      const s = await getSettings();
      setSettings(s);
      setDraft(s);
    } catch (e) {
      fail(e);
    }
  }

  useEffect(() => {
    load();
    getGithubStatus().then(setGh).catch(() => setGh(null));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="space-y-4">
      <PageHeader
        title="Ajustes"
        description="Ajustes de flota. Aplican en caliente salvo binarios, que exigen reinicio del servicio."
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{Object.keys(settings).length} claves</span>}
      />
      {justInstalled && (
        <div className="flex items-center justify-between gap-3 border border-ok/35 bg-ok/5 px-3 py-2">
          <p className="font-mono text-[10px] text-ok">
            GitHub conectado ✓ · los repos ya aparecen en “nueva app”
          </p>
          <button
            type="button"
            onClick={() => setParams({}, { replace: true })}
            className="font-mono text-[9px] uppercase tracking-[0.08em] text-muted-foreground hover:text-foreground"
          >
            cerrar
          </button>
        </div>
      )}
      <Section
        title="GitHub App"
        accent={!!gh?.configured && gh.repository_selection !== "all"}
        description="clona repos privados sin PAT · instala la App en cada org para ver sus repos"
        actions={
          gh?.configured && gh.install_url ? (
            <a
              href={gh.install_url}
              target="_blank"
              rel="noreferrer"
              className="font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:text-foreground"
            >
              administrar ↗
            </a>
          ) : undefined
        }
      >
        {!gh && <p className="font-mono text-[10px] text-muted-foreground">consultando GitHub…</p>}
        {gh && !gh.configured && (
          <div className="space-y-2">
            <p className="text-[12px] text-muted-foreground">
              Sin App conectada. Crea la GitHub App desde aquí y en el paso de instalación elige{" "}
              <b className="text-foreground">All repositories</b>. Para repos de una organización, instálala
              también en esa org.
            </p>
            {gh.setup_url ? (
              <div className="space-y-2">
                <a href={gh.setup_url} target="_blank" rel="noreferrer">
                  <Button size="sm" variant="outline">
                    crear GitHub App ↗
                  </Button>
                </a>
                {gh.local && (
                  <p className="font-mono text-[10px] text-muted-foreground">
                    modo local (localhost): GitHub no entrega webhooks aquí; el auto-deploy usa polling.
                  </p>
                )}
              </div>
            ) : (
              <p className="border border-warn/30 bg-warn/5 px-2 py-1.5 font-mono text-[10px] text-warn">
                define <b>base_domain</b> arriba para habilitar el flujo (necesita dominio público con TLS).
              </p>
            )}
          </div>
        )}
        {gh && gh.configured && (
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-px border border-border bg-border md:grid-cols-4">
              <div className="bg-card/45 px-3 py-2">
                <p className="micro">cuenta</p>
                <p className="mt-1 truncate font-mono text-[11px]">{gh.account || "—"}</p>
              </div>
              <div className="bg-card/45 px-3 py-2">
                <p className="micro">app</p>
                <p className="mt-1 truncate font-mono text-[11px]">{gh.app_slug || gh.app_id}</p>
              </div>
              <div className="bg-card/45 px-3 py-2">
                <p className="micro">repositorios</p>
                <p className="mt-1 font-mono text-[11px]">{gh.repositories ?? "—"}</p>
              </div>
              <div className="bg-card/45 px-3 py-2">
                <p className="micro">alcance</p>
                <p className={`mt-1 font-mono text-[11px] ${gh.repository_selection === "all" ? "text-ok" : gh.repository_selection ? "text-warn" : "text-muted-foreground"}`}>
                  {gh.repository_selection === "all" ? "todos" : gh.repository_selection === "selected" ? "seleccionados" : "falta instalar"}
                </p>
              </div>
            </div>
            {!gh.repository_selection ? (
              <div className="border border-border bg-card/30 px-3 py-2 text-[12px]">
                <p className="text-muted-foreground">
                  App creada. Falta instalarla en tu cuenta y elegir <b className="text-foreground">All repositories</b>.
                </p>
                {gh.install_url && (
                  <a href={gh.install_url} target="_blank" rel="noreferrer" className="mt-2 inline-block">
                    <Button size="sm" variant="outline">
                      instalar con todos los repos ↗
                    </Button>
                  </a>
                )}
              </div>
            ) : gh.repository_selection !== "all" ? (
              <div className="border border-warn/35 bg-warn/5 px-3 py-2 text-[12px]">
                <p className="font-mono text-[10px] tracking-[0.08em] text-warn uppercase">acceso parcial</p>
                <p className="mt-1 text-muted-foreground">
                  Los repos privados fuera de la selección fallarán al desplegar. Cambia a{" "}
                  <b className="text-foreground">All repositories</b> en la instalación.
                </p>
                {gh.install_url && (
                  <a href={gh.install_url} target="_blank" rel="noreferrer" className="mt-2 inline-block">
                    <Button size="sm" variant="outline">
                      cambiar acceso ↗
                    </Button>
                  </a>
                )}
              </div>
            ) : (
              <p className="font-mono text-[10px] text-ok">acceso total · repos privados habilitados</p>
            )}
            {gh.error && <p className="font-mono text-[10px] text-warn">github: {gh.error}</p>}
          </div>
        )}
      </Section>
      <Section
        title="Claves de flota"
        description="guardar valida antes de persistir · sin valores por defecto inventados"
      >
        <div className="border border-border">
          <div className="hidden grid-cols-[2rem_minmax(0,1fr)_18rem_5rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 md:grid" aria-hidden>
            <span className="micro">folio</span>
            <span className="micro">clave</span>
            <span className="micro">valor</span>
            <span />
          </div>
          {Object.entries(settings).map(([k, v], i) => (
            <div
              key={k}
              className="grid grid-cols-1 gap-2 border-b border-border/60 bg-card/35 px-3 py-2.5 last:border-0 md:grid-cols-[2rem_minmax(0,1fr)_18rem_5rem] md:items-center md:gap-3"
            >
              <span className="folio hidden md:block">{String(i + 1).padStart(2, "0")}</span>
              <div className="min-w-0">
                <p className="font-mono text-[11px]">{k}</p>
                {DESCRIPTIONS[k] && <p className="mt-0.5 text-[10px] leading-relaxed text-muted-foreground">{DESCRIPTIONS[k]}</p>}
              </div>
              <span className="tnum min-w-0 truncate font-mono text-[11px] text-muted-foreground" title={v}>
                {v === "" ? "—" : v}
              </span>
              {canWrite ? (
                <FormDialog
                  trigger={
                    <Button variant="outline" size="xs">
                      editar
                    </Button>
                  }
                  title={`Editar ${k}`}
                  description={DESCRIPTIONS[k] ?? "se valida antes de persistir"}
                  submitLabel="guardar"
                  onSubmit={async () => {
                    await setSetting(k, draft[k] ?? v);
                    await load();
                  }}
                >
                  <Field label={k} hint={PICKERS[k] ? "ruta en disco" : undefined}>
                    {PICKERS[k] ? (
                      <PathInput
                        value={draft[k] ?? v}
                        onChange={(nv) => setDraft({ ...draft, [k]: nv })}
                        mode={PICKERS[k]}
                        ariaLabel={`valor de ${k}`}
                        className="tnum h-8 min-w-0 flex-1 font-mono text-[12px]"
                      />
                    ) : (
                      <Input
                        value={draft[k] ?? v}
                        onChange={(e) => setDraft({ ...draft, [k]: e.target.value })}
                        id={`set-${k}`}
                        aria-label={`valor de ${k}`}
                        className="tnum h-8 font-mono text-[12px]"
                        autoFocus
                      />
                    )}
                  </Field>
                </FormDialog>
              ) : (
                <span className="font-mono text-[9px] text-muted-foreground">solo lectura</span>
              )}
            </div>
          ))}
          {Object.keys(settings).length === 0 && (
            <p className="bg-card/30 px-3 py-5 text-center font-mono text-[10px] text-muted-foreground">cargando claves…</p>
          )}
        </div>
      </Section>
      <p className="font-mono text-[9px] leading-relaxed text-muted-foreground/70">
        Los secretos de entorno no viven aquí: se editan por app en Entorno. Este panel solo guarda configuración de flota.
      </p>
    </div>
  );
}
