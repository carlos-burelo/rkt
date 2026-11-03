import { useEffect, useState } from "react";
import { getSettings, setSetting } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader, Section } from "../../shared/components/patterns";
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="space-y-4">
      <PageHeader
        title="Ajustes"
        description="Ajustes de flota. Aplican en caliente salvo binarios, que exigen reinicio del servicio."
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{Object.keys(settings).length} claves</span>}
      />
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
              {PICKERS[k] ? (
                <PathInput
                  value={draft[k] ?? v}
                  onChange={(nv) => setDraft({ ...draft, [k]: nv })}
                  mode={PICKERS[k]}
                  disabled={!canWrite}
                  ariaLabel={`valor de ${k}`}
                  className="tnum h-7 min-w-0 flex-1 font-mono text-[11px]"
                />
              ) : (
                <Input
                  value={draft[k] ?? v}
                  onChange={(e) => setDraft({ ...draft, [k]: e.target.value })}
                  id={`set-${k}`}
                  disabled={!canWrite}
                  aria-label={`valor de ${k}`}
                  className="tnum h-7 font-mono text-[11px]"
                />
              )}
              {canWrite ? (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setSetting(k, draft[k] ?? v).then(load).catch(fail);
                  }}
                >
                  guardar
                </Button>
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
