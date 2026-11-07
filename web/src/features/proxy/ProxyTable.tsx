import { useMemo } from "react";
import { StateRail } from "../../shared/components/ui";
import { EmptyState } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import { useProxy } from "./proxy.store";
import { buildLinks } from "./graph";

// Tabla host→target con backend y cert. Filtros del store; reutilizable.
export function ProxyTable() {
  const { t } = useI18n();
  const rows = useProxy((s) => s.rows);
  const fState = useProxy((s) => s.fState);
  const fTls = useProxy((s) => s.fTls);
  const selected = useProxy((s) => s.selected);

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
  const visibleRows = (selected ? rows?.filter((r) => r.hosts.includes(selected)) : rows) ?? [];

  if (!rows) return null;
  return (
    <div className="border border-border" role="table" aria-label={t("serve.proxy.tableAria")}>
      <div
        className="hidden grid-cols-[minmax(0,1fr)_8rem_auto] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 sm:grid"
        aria-hidden
      >
        <span className="micro">{t("serve.proxy.colHostTarget")}</span>
        <span className="micro">{t("serve.field.backend")}</span>
        <span className="micro text-right">{t("serve.field.cert")}</span>
      </div>
      {visibleRows
        .filter((r) => {
          const l = byHost.get(r.hosts[0]);
          return !l || filteredLinks.includes(l) || r.hosts.some((h) => filteredLinks.some((f) => f.host === h));
        })
        .map((r) => (
          <div
            key={r.hosts.join(",") + r.target}
            className="relative grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-b border-border/70 bg-card/35 py-2.5 pr-3 pl-3 last:border-0 sm:grid-cols-[minmax(0,1fr)_8rem_auto]"
            role="row"
          >
            <StateRail status={r.ok ? "ok" : "failed"} />
            <div className="min-w-0">
              <p className="flex min-w-0 items-center gap-1.5 font-mono text-[12px]">
                <span className="truncate">{r.hosts.join(", ")}</span>
                {r.dns_ok?.some((v) => !v) && (
                  <span
                    className="shrink-0 font-mono text-[9px] tracking-[0.08em] uppercase text-warn"
                    title={t("serve.proxy.dnsTitle")}
                  >
                    {t("serve.proxy.dnsUnresolved")}
                  </span>
                )}
              </p>
              <p className="tnum mt-0.5 truncate font-mono text-[10px] text-muted-foreground">→ {r.target}</p>
            </div>
            <span
              className={`tnum hidden font-mono text-[10px] tracking-[0.06em] uppercase sm:block ${
                r.ok ? "text-ok" : "text-bad"
              }`}
            >
              {r.ok ? t("serve.proxy.backendOk") : t("serve.proxy.backendDown")}
            </span>
            <span className="flex flex-wrap justify-end gap-1">
              {r.tls.map((tls, i) => (
                <span
                  key={i}
                  className={`tnum inline-flex h-5 items-center border px-1.5 font-mono text-[10px] tracking-[0.06em] uppercase ${
                    tls === "off"
                      ? "border-border text-muted-foreground"
                      : "border-info/30 bg-info/10 text-info"
                  }`}
                >
                  {tls === "off" ? t("serve.tls.none") : t("serve.tls.value", { version: tls })}
                </span>
              ))}
            </span>
          </div>
        ))}
      {rows.length === 0 && (
        <EmptyState
          title={t("serve.proxy.emptyTitle")}
          hint={t("serve.proxy.emptyHint", { domains: t("nav.routes") })}
        />
      )}
    </div>
  );
}
