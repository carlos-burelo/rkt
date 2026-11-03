import { useEffect, useState } from "react";
import { getSettings, setSetting } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader } from "../../shared/components/patterns";
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
      <div className="rounded-lg border bg-card p-4">
        {Object.entries(settings).map(([k, v]) => (
          <div key={k} className="border-b py-3 last:border-0">
            <div className="flex items-center gap-2">
              <span className="w-40 shrink-0 font-mono text-xs">{k}</span>
              <Input
                defaultValue={v}
                id={`set-${k}`}
                disabled={!canWrite}
                className="flex-1 font-mono text-xs"
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
            {DESCRIPTIONS[k] && <p className="mt-1 text-[11px] text-muted-foreground">{DESCRIPTIONS[k]}</p>}
          </div>
        ))}
        {Object.keys(settings).length === 0 && <p className="text-xs text-muted-foreground">cargando…</p>}
      </div>
    </div>
  );
}
