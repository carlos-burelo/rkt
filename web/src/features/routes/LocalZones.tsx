import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Section } from "../../shared/components/patterns";
import { Dot } from "../../shared/components/ui";
import { useUI } from "../../app/ui";
import { useFleet } from "../overview/hooks";
import { addRoute, delRoute, listZones, ZoneGroup, Zones } from "./api";

const SLUG = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/;

// Zonas locales: dominio base + subdominios, como un panel DNS.
// Todo va a /api/v1/routes (una ruta por host); aquí solo se agrupa.
export function LocalZones({ onChanged }: { onChanged: () => void }) {
  const fail = useUI((u) => u.fail);
  const { metas } = useFleet();
  const [zones, setZones] = useState<Zones | null>(null);
  const [prefix, setPrefix] = useState("");
  const [suffix, setSuffix] = useState("localhost");
  const [target, setTarget] = useState("http://127.0.0.1:3000");
  const [subs, setSubs] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  const load = () => listZones().then(setZones).catch(fail);
  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // target por defecto desde la primera unidad, si existe
  useEffect(() => {
    if (metas[0]?.port) setTarget((t) => (t === "http://127.0.0.1:3000" ? `http://127.0.0.1:${metas[0].port}` : t));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [metas.length]);

  function createZone() {
    const p = prefix.trim().toLowerCase();
    if (!SLUG.test(p)) {
      fail("subdominio inválido: minúsculas, números y guiones");
      return;
    }
    setBusy(true);
    addRoute([`${p}.${suffix}`], target.trim())
      .then(() => {
        setPrefix("");
        return Promise.all([load(), Promise.resolve(onChanged())]);
      })
      .catch(fail)
      .finally(() => setBusy(false));
  }

  function addSub(g: ZoneGroup) {
    const s = (subs[g.base] ?? "").trim().toLowerCase();
    if (!SLUG.test(s)) {
      fail("subdominio inválido: minúsculas, números y guiones");
      return;
    }
    setBusy(true);
    addRoute([`${s}.${g.base}`], g.target || target.trim())
      .then(() => {
        setSubs({ ...subs, [g.base]: "" });
        return Promise.all([load(), Promise.resolve(onChanged())]);
      })
      .catch(fail)
      .finally(() => setBusy(false));
  }

  function remove(host: string) {
    setBusy(true);
    delRoute(host)
      .then(() => Promise.all([load(), Promise.resolve(onChanged())]))
      .catch(fail)
      .finally(() => setBusy(false));
  }

  const suffixes = zones?.suffixes ?? ["localhost", "test"];

  return (
    <Section
      title="Zonas locales"
      description="dominio local + subdominios · .localhost resuelve sin editar hosts"
      actions={<span className="micro">{zones?.groups.length ?? 0} zonas</span>}
    >
      {suffixes.filter((s) => !s.includes(".")).length > 0 && (
        <div className="mb-3 flex flex-col gap-2 sm:flex-row sm:items-end">
          <label className="field-label min-w-0 flex-1">
            dominio
            <Input
              value={prefix}
              onChange={(e) => setPrefix(e.target.value)}
              placeholder="mi-app"
              className="h-7 font-mono text-[11px]"
              aria-label="nombre del dominio local"
            />
          </label>
          <label className="field-label w-32 shrink-0">
            zona
            <select
              value={suffix}
              onChange={(e) => setSuffix(e.target.value)}
              className="h-7 w-full border border-input bg-transparent px-2 font-mono text-[11px]"
              aria-label="zona local"
            >
              {suffixes.map((s) => (
                <option key={s} value={s}>
                  .{s}
                </option>
              ))}
            </select>
          </label>
          <label className="field-label min-w-0 flex-1">
            target
            <Input
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              placeholder="http://127.0.0.1:3000"
              className="h-7 font-mono text-[11px]"
              aria-label="target del dominio"
            />
          </label>
          <Button size="sm" onClick={createZone} disabled={busy}>
            <Plus className="h-3 w-3" /> crear zona
          </Button>
        </div>
      )}

      <div className="border border-border">
        {(zones?.groups ?? []).length === 0 && (
          <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">
            sin zonas locales todavía. crea una arriba (ej. mi-app.localhost) y añade subdominios.
          </p>
        )}
        {(zones?.groups ?? []).map((g) => (
          <div key={g.base} className="border-b border-border/60 last:border-0">
            <div className="flex flex-wrap items-center gap-2 bg-card/40 px-3 py-2">
              <Dot status={g.target ? "ok" : "stopped"} />
              <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{g.base}</span>
              <span className="tnum hidden font-mono text-[9px] text-muted-foreground sm:block">
                {g.target || "sin apex"}
              </span>
              {g.target && (
                <Button
                  variant="ghost"
                  size="xs"
                  className="text-muted-foreground hover:text-bad"
                  onClick={() => remove(g.base)}
                  aria-label={`borrar ${g.base}`}
                  title="borrar dominio"
                >
                  <Trash2 className="h-3 w-3" />
                </Button>
              )}
              <span className="inline-flex items-center gap-1">
                <Input
                  value={subs[g.base] ?? ""}
                  onChange={(e) => setSubs({ ...subs, [g.base]: e.target.value })}
                  placeholder="subdominio"
                  className="h-6 w-32 font-mono text-[10px]"
                  aria-label={`nuevo subdominio de ${g.base}`}
                />
                <Button variant="outline" size="xs" onClick={() => addSub(g)} disabled={busy}>
                  <Plus className="h-3 w-3" />
                </Button>
              </span>
            </div>
            {g.hosts
              .filter((h) => !h.is_base)
              .map((h) => (
                <div key={h.host} className="flex items-center gap-2 border-t border-border/40 py-1.5 pr-3 pl-8">
                  <span aria-hidden className="text-[9px] text-muted-foreground/40">└</span>
                  <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{h.host}</span>
                  <span className="tnum hidden font-mono text-[9px] text-muted-foreground sm:block">{h.target}</span>
                  <Button
                    variant="ghost"
                    size="xs"
                    className="text-muted-foreground hover:text-bad"
                    onClick={() => remove(h.host)}
                    aria-label={`borrar ${h.host}`}
                    title="borrar subdominio"
                  >
                    <Trash2 className="h-3 w-3" />
                  </Button>
                </div>
              ))}
          </div>
        ))}
      </div>
    </Section>
  );
}
