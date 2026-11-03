import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { ArrowRight, X } from "lucide-react";
import { fetchProxy, ProxyRoute } from "./api";
import { buildLinks } from "./graph";
import { RadialMap } from "./ProxyMap";
import { StateRail } from "../../shared/components/ui";
import { PageHeader, EmptyState, Segmented } from "../../shared/components/patterns";
import { useUI } from "../../app/ui";

// Proxy: lo que Vercel esconde. Cada host, a dónde va, si el backend
// responde (dial) y estado del cert. Solo lectura.
export function Proxy() {
  const fail = useUI((u) => u.fail);
  const [rows, setRows] = useState<ProxyRoute[] | null>(null);
  const [fState, setFState] = useState<"all" | "ok" | "down">("all");
  const [fTls, setFTls] = useState<"all" | "tls" | "off">("all");
  const [selected, setSelected] = useState<string | null>(null);

  useEffect(() => {
    fetchProxy().then(setRows).catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
  const visibleRows = (selected ? rows?.filter((r) => r.hosts.includes(selected)) : rows) ?? [];

  return (
    <div className="space-y-4">
      <PageHeader
        title="Proxy"
        description="Mapa real por ruta: dial TCP al backend y cert ACME. Toca un host para aislar su haz. Solo lectura."
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {links.filter((l) => l.ok).length}/{links.length} en ruta
          </span>
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Segmented
              value={fState}
              onChange={setFState}
              label="estado del backend"
              items={[
                { value: "all", label: "todos" },
                { value: "ok", label: "en ruta" },
                { value: "down", label: "varados" },
              ]}
            />
            <Segmented
              value={fTls}
              onChange={setFTls}
              label="estado tls"
              items={[
                { value: "all", label: "tls todo" },
                { value: "tls", label: "con tls" },
                { value: "off", label: "sin tls" },
              ]}
            />
            <Link
              to="/routes"
              className="inline-flex items-center gap-1 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:text-foreground"
            >
              gestionar <ArrowRight className="h-3 w-3" />
            </Link>
          </div>
        }
      />
      {!rows && <p className="font-mono text-[10px] text-muted-foreground">cargando mapa…</p>}
      {rows && (
        <>
          <RadialMap links={filteredLinks} selected={selected} onSelect={setSelected} />
          {selected && byHost.get(selected) && (
            <button
              onClick={() => setSelected(null)}
              className="tnum inline-flex w-fit items-center gap-1.5 border border-border px-2 py-1 font-mono text-[9px] text-muted-foreground hover:border-input hover:text-foreground"
            >
              <X className="h-3 w-3" /> limpiar filtro: {selected}
            </button>
          )}

          <div className="border border-border" role="table" aria-label="rutas del proxy">
            <div
              className="hidden grid-cols-[minmax(0,1fr)_8rem_auto] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 sm:grid"
              aria-hidden
            >
              <span className="micro">host → target</span>
              <span className="micro">backend</span>
              <span className="micro text-right">cert</span>
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
                    <p className="truncate font-mono text-[11px]">{r.hosts.join(", ")}</p>
                    <p className="tnum mt-0.5 truncate font-mono text-[9px] text-muted-foreground">→ {r.target}</p>
                  </div>
                  <span
                    className={`tnum hidden font-mono text-[9px] tracking-[0.06em] uppercase sm:block ${
                      r.ok ? "text-ok" : "text-bad"
                    }`}
                  >
                    {r.ok ? "backend ok" : "backend caído"}
                  </span>
                  <span className="flex flex-wrap justify-end gap-1">
                    {r.tls.map((t, i) => (
                      <span
                        key={i}
                        className={`tnum inline-flex h-5 items-center border px-1.5 font-mono text-[9px] tracking-[0.06em] uppercase ${
                          t === "off"
                            ? "border-border text-muted-foreground"
                            : "border-info/30 bg-info/10 text-info"
                        }`}
                      >
                        {t === "off" ? "sin tls" : `tls ${t}`}
                      </span>
                    ))}
                  </span>
                </div>
              ))}
            {rows.length === 0 && <EmptyState title="Sin routes" hint="Agrega tu primer dominio en Domains." />}
          </div>
        </>
      )}
    </div>
  );
}
