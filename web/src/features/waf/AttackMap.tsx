import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Section } from "../../shared/components/patterns";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import { fetchAttacks, updateGeoip, type AttackCount, type AttacksState } from "./api";

const WINDOWS = [24, 72, 168] as const;

// Bandera por código ISO (indicadores regionales); "??" = sin geolocalizar.
function flag(cc: string): string {
  if (!/^[A-Z]{2}$/.test(cc)) return "🌐";
  return String.fromCodePoint(...[...cc].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
}

function Bars({ rows, icon }: { rows: AttackCount[]; icon?: (k: string) => string }) {
  const max = Math.max(1, ...rows.map((r) => r.count));
  return (
    <ul className="space-y-1">
      {rows.map((r) => (
        <li key={r.key} className="grid grid-cols-[minmax(0,10rem)_1fr_3rem] items-center gap-2 font-mono text-[11px]">
          <span className="truncate" title={r.key}>
            {icon ? `${icon(r.key)} ` : ""}
            {r.key}
          </span>
          <span className="h-2 bg-secondary/40">
            <span className="block h-full bg-warn/70" style={{ width: `${(r.count / max) * 100}%` }} />
          </span>
          <span className="tnum text-right text-muted-foreground">{r.count}</span>
        </li>
      ))}
    </ul>
  );
}

// Mapa de ataques: de dónde vienen los bloqueos del WAF (país, IP, regla,
// hora). Países por base geoip local opt-in; sin ella salen como "??".
export function AttackMap() {
  const { t } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const [hours, setHours] = useState<number>(24);
  const [data, setData] = useState<AttacksState | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    fetchAttacks(hours)
      .then(setData)
      .catch(() => setData(null));
  }, [hours]);

  useEffect(() => {
    load();
    const id = setInterval(load, 30000);
    return () => clearInterval(id);
  }, [load]);

  const installGeo = async () => {
    setBusy(true);
    try {
      await updateGeoip();
      toast.success(t("serve.attacks.geoipOk"));
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "error");
    } finally {
      setBusy(false);
    }
  };

  const s = data?.summary;
  const peak = Math.max(1, ...(s?.hourly ?? []).map((h) => h.count));

  return (
    <Section
      title={t("serve.attacks.title")}
      description={t("serve.attacks.desc", { total: s?.total ?? 0, hours })}
      actions={
        <div className="flex items-center gap-1">
          {WINDOWS.map((w) => (
            <Button key={w} size="xs" variant={w === hours ? "default" : "outline"} onClick={() => setHours(w)}>
              {w}h
            </Button>
          ))}
        </div>
      }
    >
      {data && !data.geoip_ready && (
        <div className="mb-3 flex items-center gap-3 border border-warn/30 bg-warn/5 px-3 py-2 text-[11px] text-warn">
          <span className="flex-1">{t("serve.attacks.noGeoip")}</span>
          {canWrite && (
            <Button size="xs" variant="outline" disabled={busy} onClick={installGeo}>
              {busy ? t("serve.attacks.downloading") : t("serve.attacks.installGeoip")}
            </Button>
          )}
        </div>
      )}
      {s && s.total === 0 && <p className="font-mono text-[10px] text-muted-foreground">{t("serve.attacks.none")}</p>}
      {s && s.total > 0 && (
        <div className="space-y-4">
          <div className="flex h-12 items-end gap-px" aria-hidden>
            {s.hourly.map((h) => (
              <span key={h.key} title={`${h.key} · ${h.count}`} className="flex-1 bg-warn/60" style={{ height: `${(h.count / peak) * 100}%` }} />
            ))}
          </div>
          <div className="grid gap-4 md:grid-cols-3">
            <div>
              <p className="micro mb-1.5">{t("serve.attacks.countries")}</p>
              <Bars rows={s.countries} icon={flag} />
            </div>
            <div>
              <p className="micro mb-1.5">{t("serve.attacks.ips")}</p>
              <Bars rows={s.ips} />
            </div>
            <div>
              <p className="micro mb-1.5">{t("serve.attacks.rules")}</p>
              <Bars rows={s.rules} />
            </div>
          </div>
        </div>
      )}
    </Section>
  );
}
