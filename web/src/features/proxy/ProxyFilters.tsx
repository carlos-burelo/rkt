import { SingleFilter } from "../../shared/components/filters";
import { useI18n } from "../../shared/i18n";
import { useProxy } from "./proxy.store";

// Filtros de estado/TLS del mapa proxy. Leen el store: misma selección
// en Dominios y en Diagnóstico.
export function ProxyFilters({ ok, total }: { ok: number; total: number }) {
  const { t } = useI18n();
  const fState = useProxy((s) => s.fState);
  const fTls = useProxy((s) => s.fTls);
  const st = useProxy.getState();
  return (
    <>
      <SingleFilter
        value={fState}
        onChange={st.setFState}
        label={t("serve.proxy.filterState")}
        items={[
          { value: "all" as const, label: t("common.all") },
          { value: "ok" as const, label: t("serve.proxy.onRoute") },
          { value: "down" as const, label: t("serve.proxy.stranded") },
        ]}
      />
      <SingleFilter
        value={fTls}
        onChange={st.setFTls}
        label={t("serve.proxy.filterTls")}
        items={[
          { value: "all" as const, label: t("serve.proxy.tlsAll") },
          { value: "tls" as const, label: t("serve.proxy.withTls") },
          { value: "off" as const, label: t("serve.tls.none") },
        ]}
      />
      <span className="tnum font-mono text-[10px] text-muted-foreground">
        {t("serve.proxy.meta", { ok, total })}
      </span>
    </>
  );
}
