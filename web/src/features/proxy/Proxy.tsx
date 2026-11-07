import { useEffect, useMemo } from "react";
import { Link } from "react-router";
import { ArrowRight, X } from "lucide-react";
import { useProxy } from "./proxy.store";
import { ProxyFilters } from "./ProxyFilters";
import { ProxyTable } from "./ProxyTable";
import { buildLinks } from "./graph";
import { RadialMap } from "./ProxyMap";
import { PageHeader } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";

// Proxy: lo que Vercel esconde. Cada host, a dónde va, si el backend
// responde (dial) y estado del cert. Solo lectura. Dentro de Dominios se
// renderiza como pestaña de diagnóstico (embedded).
export function Proxy({ embedded = false }: { embedded?: boolean }) {
  const { t } = useI18n();
  const rows = useProxy((s) => s.rows);
  const err = useProxy((s) => s.err);
  const fState = useProxy((s) => s.fState);
  const fTls = useProxy((s) => s.fTls);
  const selected = useProxy((s) => s.selected);

  useEffect(() => {
    useProxy.getState().load(false);
  }, []);

  const links = useMemo(() => (rows ? buildLinks(rows) : []), [rows]);
  const filteredLinks = useMemo(
    () =>
      links.filter(
        (l) =>
          (fState === "all" || (fState === "ok" ? l.ok : !l.ok)) &&
          (fTls === "all" || (fTls === "tls" ? l.tls !== "off" : l.tls === "off")),
      ),
    [links, fState, fTls],
  );
  const byHost = useMemo(() => new Map(links.map((l) => [l.host, l])), [links]);

  return (
    <div className={embedded ? "space-y-3" : "space-y-4"}>
      {!embedded && (
        <PageHeader
          title={t("nav.proxy")}
          description={t("serve.proxy.desc")}
          meta={
            <span className="tnum font-mono text-[10px] text-muted-foreground">
              {t("serve.proxy.meta", {
                ok: links.filter((l) => l.ok).length,
                total: links.length,
              })}
            </span>
          }
          actions={
            <div className="flex flex-wrap items-center gap-2">
              <ProxyFilters ok={links.filter((l) => l.ok).length} total={links.length} />
              <Link
                to="/routes"
                className="inline-flex items-center gap-1 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:text-foreground"
              >
                {t("serve.proxy.manage")} <ArrowRight className="h-3 w-3" />
              </Link>
            </div>
          }
        />
      )}
      {embedded && (
        <div className="flex flex-wrap items-center gap-2">
          <ProxyFilters ok={links.filter((l) => l.ok).length} total={links.length} />
        </div>
      )}
      {err !== "" && <p className="font-mono text-[10px] text-bad">{err}</p>}
      {!rows && <p className="font-mono text-[10px] text-muted-foreground">{t("serve.proxy.loading")}</p>}
      {rows && (
        <>
          <RadialMap links={filteredLinks} selected={selected} onSelect={(h) => useProxy.getState().setSelected(h)} />
          {selected && byHost.get(selected) && (
            <button
              onClick={() => useProxy.getState().setSelected(null)}
              className="tnum inline-flex w-fit items-center gap-1.5 border border-border px-2 py-1 font-mono text-[9px] text-muted-foreground hover:border-input hover:text-foreground"
            >
              <X className="h-3 w-3" /> {t("serve.proxy.clearFilter", { host: selected })}
            </button>
          )}

          <ProxyTable />
        </>
      )}
    </div>
  );
}
