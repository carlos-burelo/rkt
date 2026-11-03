import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Section } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
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

  async function createZone() {
    const p = prefix.trim().toLowerCase();
    if (!SLUG.test(p)) {
      throw new Error("subdominio inválido: minúsculas, números y guiones");
    }
    await addRoute([`${p}.${suffix}`], target.trim());
    setPrefix("");
    await load();
    onChanged();
  }

  async function addSub(g: ZoneGroup) {
    const s = (subs[g.base] ?? "").trim().toLowerCase();
    if (!SLUG.test(s)) {
      throw new Error("subdominio inválido: minúsculas, números y guiones");
    }
    await addRoute([`${s}.${g.base}`], g.target || target.trim());
    setSubs({ ...subs, [g.base]: "" });
    await load();
    onChanged();
  }

  function remove(host: string) {
    delRoute(host)
      .then(() => Promise.all([load(), Promise.resolve(onChanged())]))
      .catch(fail);
  }

  const suffixes = zones?.suffixes ?? ["localhost", "test"];

  return (
    <Section
      title="Zonas locales"
      description="dominio local + subdominios · .localhost resuelve sin editar hosts"
      actions={
        <div className="flex items-center gap-2">
          <span className="micro">{zones?.groups.length ?? 0} zonas</span>
          {suffixes.filter((s) => !s.includes(".")).length > 0 && (
            <FormDialog
              trigger={
                <Button size="xs" variant="outline">
                  <Plus className="h-3 w-3" /> nueva zona
                </Button>
              }
              title="Nueva zona local"
              description="crea la ruta del dominio · .localhost resuelve solo"
              submitLabel="crear"
              canSubmit={SLUG.test(prefix.trim().toLowerCase()) && target.trim() !== ""}
              onSubmit={createZone}
            >
              <div className="grid grid-cols-2 gap-3">
                <Field label="dominio" hint="sin punto">
                  <Input
                    value={prefix}
                    onChange={(e) => setPrefix(e.target.value)}
                    placeholder="mi-app"
                    className="h-8 font-mono text-[12px]"
                    aria-label="nombre del dominio local"
                    autoFocus
                  />
                </Field>
                <Field label="zona">
                  <select
                    value={suffix}
                    onChange={(e) => setSuffix(e.target.value)}
                    className="h-8 w-full border border-input bg-transparent px-2 font-mono text-[12px]"
                    aria-label="zona local"
                  >
                    {suffixes.map((s) => (
                      <option key={s} value={s}>
                        .{s}
                      </option>
                    ))}
                  </select>
                </Field>
              </div>
              <Field label="target" hint="http(s)://127.0.0.1:puerto">
                <Input
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  placeholder="http://127.0.0.1:3000"
                  className="tnum h-8 font-mono text-[12px]"
                  aria-label="target del dominio"
                />
              </Field>
            </FormDialog>
          )}
        </div>
      }
    >
      <div className="border border-border">
        {(zones?.groups ?? []).length === 0 && (
          <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">
            sin zonas locales todavía. crea una con «nueva zona» (ej. mi-app.localhost) y añade subdominios.
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
                  variant="destructive"
                  size="icon-xs"
                  onClick={() => remove(g.base)}
                  aria-label={`borrar ${g.base}`}
                  title="borrar dominio"
                >
                  <Trash2 className="h-3 w-3" />
                </Button>
              )}
              <FormDialog
                trigger={
                  <Button variant="outline" size="xs" title={`añadir subdominio a ${g.base}`}>
                    <Plus className="h-3 w-3" /> subdominio
                  </Button>
                }
                title={`Nuevo subdominio de ${g.base}`}
                description="se crea la ruta y el proxy recarga al instante"
                submitLabel="crear"
                canSubmit={SLUG.test((subs[g.base] ?? "").trim().toLowerCase())}
                onSubmit={() => addSub(g)}
              >
                <Field label="subdominio" hint={`se añade .${g.base}`}>
                  <Input
                    value={subs[g.base] ?? ""}
                    onChange={(e) => setSubs({ ...subs, [g.base]: e.target.value })}
                    placeholder="api"
                    className="h-8 font-mono text-[12px]"
                    aria-label={`nuevo subdominio de ${g.base}`}
                    autoFocus
                  />
                </Field>
                {!g.target && (
                  <p className="font-mono text-[9px] text-muted-foreground">
                    esta zona no tiene apex; usará el target por defecto {target}
                  </p>
                )}
              </FormDialog>
            </div>
            {g.hosts
              .filter((h) => !h.is_base)
              .map((h) => (
                <div key={h.host} className="flex items-center gap-2 border-t border-border/40 py-1.5 pr-3 pl-8">
                  <span aria-hidden className="text-[9px] text-muted-foreground/40">└</span>
                  <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{h.host}</span>
                  <span className="tnum hidden font-mono text-[9px] text-muted-foreground sm:block">{h.target}</span>
                  <Button
                    variant="destructive"
                    size="icon-xs"
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
