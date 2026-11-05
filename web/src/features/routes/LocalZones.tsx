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
import { useI18n } from "../../shared/i18n";

const SLUG = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/;

// Zonas locales: dominio base + subdominios, como un panel DNS.
// Todo va a /api/v1/routes (una ruta por host); aquí solo se agrupa.
export function LocalZones({ onChanged }: { onChanged: () => void }) {
  const { t, tn } = useI18n();
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
    if (metas[0]?.port)
      setTarget((prev) => (prev === "http://127.0.0.1:3000" ? `http://127.0.0.1:${metas[0].port}` : prev));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [metas.length]);

  async function createZone() {
    const p = prefix.trim().toLowerCase();
    if (!SLUG.test(p)) {
      throw new Error(t("serve.zones.invalidSub"));
    }
    await addRoute([`${p}.${suffix}`], target.trim());
    setPrefix("");
    await load();
    onChanged();
  }

  async function addSub(g: ZoneGroup) {
    const s = (subs[g.base] ?? "").trim().toLowerCase();
    if (!SLUG.test(s)) {
      throw new Error(t("serve.zones.invalidSub"));
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
      title={t("serve.zones.title")}
      description={t("serve.zones.desc")}
      actions={
        <div className="flex items-center gap-2">
          <span className="micro">{tn("serve.zones.count", zones?.groups.length ?? 0)}</span>
          {suffixes.filter((s) => !s.includes(".")).length > 0 && (
            <FormDialog
              trigger={
                <Button size="xs" variant="outline">
                  <Plus className="h-3 w-3" /> {t("serve.zones.new")}
                </Button>
              }
              title={t("serve.zones.newTitle")}
              description={t("serve.zones.newDesc")}
              submitLabel={t("common.create")}
              canSubmit={SLUG.test(prefix.trim().toLowerCase()) && target.trim() !== ""}
              onSubmit={createZone}
            >
              <div className="grid grid-cols-2 gap-3">
                <Field label={t("serve.zones.domain")} hint={t("serve.zones.domainHint")}>
                  <Input
                    value={prefix}
                    onChange={(e) => setPrefix(e.target.value)}
                    placeholder={t("serve.zones.domainPlaceholder")}
                    className="h-8 font-mono text-[12px]"
                    aria-label={t("serve.zones.domainAria")}
                    autoFocus
                  />
                </Field>
                <Field label={t("serve.zones.zone")}>
                  <select
                    value={suffix}
                    onChange={(e) => setSuffix(e.target.value)}
                    className="h-8 w-full border border-input bg-transparent px-2 font-mono text-[12px]"
                    aria-label={t("serve.zones.zoneAria")}
                  >
                    {suffixes.map((s) => (
                      <option key={s} value={s}>
                        .{s}
                      </option>
                    ))}
                  </select>
                </Field>
              </div>
              <Field label={t("serve.field.target")} hint={t("serve.field.targetHint")}>
                <Input
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  placeholder={t("serve.routes.targetPlaceholder")}
                  className="tnum h-8 font-mono text-[12px]"
                  aria-label={t("serve.zones.targetAria")}
                />
              </Field>
            </FormDialog>
          )}
        </div>
      }
    >
      <div className="border border-border">
        {(zones?.groups ?? []).length === 0 && (
          <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">{t("serve.zones.empty")}</p>
        )}
        {(zones?.groups ?? []).map((g) => (
          <div key={g.base} className="border-b border-border/60 last:border-0">
            <div className="flex flex-wrap items-center gap-2 bg-card/40 px-3 py-2">
              <Dot status={g.target ? "ok" : "stopped"} />
              <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{g.base}</span>
              <span className="tnum hidden font-mono text-[9px] text-muted-foreground sm:block">
                {g.target || t("serve.zones.noApex")}
              </span>
              {g.target && (
                <Button
                  variant="destructive"
                  size="icon-xs"
                  onClick={() => remove(g.base)}
                  aria-label={t("serve.zones.deleteAria", { host: g.base })}
                  title={t("serve.zones.deleteDomain")}
                >
                  <Trash2 className="h-3 w-3" />
                </Button>
              )}
              <FormDialog
                trigger={
                  <Button variant="outline" size="xs" title={t("serve.zones.addSubTitle", { host: g.base })}>
                    <Plus className="h-3 w-3" /> {t("serve.zones.addSub")}
                  </Button>
                }
                title={t("serve.zones.newSubTitle", { host: g.base })}
                description={t("serve.zones.newSubDesc")}
                submitLabel={t("common.create")}
                canSubmit={SLUG.test((subs[g.base] ?? "").trim().toLowerCase())}
                onSubmit={() => addSub(g)}
              >
                <Field label={t("serve.zones.sub")} hint={t("serve.zones.subHint", { base: g.base })}>
                  <Input
                    value={subs[g.base] ?? ""}
                    onChange={(e) => setSubs({ ...subs, [g.base]: e.target.value })}
                    placeholder={t("serve.zones.subPlaceholder")}
                    className="h-8 font-mono text-[12px]"
                    aria-label={t("serve.zones.subAria", { base: g.base })}
                    autoFocus
                  />
                </Field>
                {!g.target && (
                  <p className="font-mono text-[9px] text-muted-foreground">
                    {t("serve.zones.noApexHint", { target })}
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
                    aria-label={t("serve.zones.deleteAria", { host: h.host })}
                    title={t("serve.zones.deleteSub")}
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
