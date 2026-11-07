import { Segmented } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import { useProxy } from "./proxy.store";

// Filtros de estado/TLS del mapa proxy. Leen el store: misma selección
// en Dominios y en Diagnóstico.
export function ProxyFilters({ ok, total }: { ok: number; total: number }) {
  const { t } = useI18n();
  const fState = useProxy((s) => s.fState);
  const fTls = useProxy((s) => s.fTls);
  return (
    <>
      <Segmented
        value={fState}
        onChange={(v) => useProxy.getState().setFState(v)}
        label={t("serve.proxy.filterState")}
        items={[
          { value: "all", label: t("common.all") },
          { value: "ok", label: t("serve.proxy.onRoute") },
          { value: "down", label: t("serve.proxy.stranded") },
        ]}
      />
      <Segmented
        value={fTls}
        onChange={(v) => useProxy.getState().setFTls(v)}
        label={t("serve.proxy.filterTls")}
        items={[
          { value: "all", label: t("serve.proxy.tlsAll") },
          { value: "tls", label: t("serve.proxy.withTls") },
          { value: "off", label: t("serve.tls.none") },
        ]}
      />
      <span className="tnum font-mono text-[10px] text-muted-foreground">
        {t("serve.proxy.meta", { ok, total })}
      </span>
    </>
  );
}
