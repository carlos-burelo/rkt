import { useEffect, useState } from "react";
import { getSettings, setSetting } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader, Section } from "../../shared/components/patterns";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

const DESCRIPTIONS: Record<string, string> = {
  node_bin: "Binario node para presets next/node.",
  python_bin: "Binario python para preset python.",
  base_domain: "Dominio base para sugerir <app>.<base>.",
  port_pool: "Rango de puertos locales (ej 3000-3999).",
  site_name: "Nombre visible en sidebar y topbar.",
  quota_reqs: "Tope requests 30d para Usage (0 = sin tope).",
  quota_bytes: "Tope bytes 30d para Usage (0 = sin tope).",
};

export function Settings() {
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const [settings, setSettings] = useState<Record<string, string>>({});

  async function load() {
    try {
      setSettings(await getSettings());
    } catch (e) {
      fail(e);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="space-y-3">
      <PageHeader title="Settings" description="Ajustes de flota. Aplican en caliente salvo binarios." />
      <Section title={`Claves · ${Object.keys(settings).length}`} description="Guardar valida antes de persistir. Binarios exigen restart.">
        {Object.entries(settings).map(([k, v]) => (
          <div key={k} className="flex items-center gap-3 border-b border-border/60 py-3 last:border-0">
            <div className="min-w-0 flex-1">
              <p className="font-mono text-xs">{k}</p>
              {DESCRIPTIONS[k] && <p className="mt-0.5 text-[11px] text-muted-foreground">{DESCRIPTIONS[k]}</p>}
            </div>
            <Input
              key={v}
              defaultValue={v}
              id={`set-${k}`}
              disabled={!canWrite}
              className="tnum w-56 shrink-0 font-mono text-xs"
            />
            {canWrite && (
              <Button
                variant="outline" size="sm"
                onClick={() => {
                  const el = document.getElementById(`set-${k}`) as HTMLInputElement;
                  setSetting(k, el.value).then(load).catch(fail);
                }}
              >
                guardar
              </Button>
            )}
          </div>
        ))}
        {Object.keys(settings).length === 0 && <p className="text-xs text-muted-foreground">cargando…</p>}
      </Section>
    </div>
  );
}
