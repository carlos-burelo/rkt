import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { fetchProxy, ProxyRoute } from "./api";
import { buildLinks } from "./graph";
import { RadialMap } from "./ProxyMap";
import { Dot } from "../../shared/components/ui";
import { Badge } from "@/components/ui/badge";
import {
  Select, SelectValue, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
import { useUI } from "../../app/ui";

// Proxy: lo que Vercel esconde. Cada host, a dónde va, si el backend
// responde (dial) y estado del cert. Solo lectura.
export function Proxy() {
  const fail = useUI((u) => u.fail);
  const [rows, setRows] = useState<ProxyRoute[] | null>(null);
  const [fState, setFState] = useState("all");
  const [fTls, setFTls] = useState("all");
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

  return (
    <div className="space-y-3">
      <PageHeader
        title="Proxy"
        description="Mapa real por ruta (dial TCP al backend + cert ACME). Toca un host para aislar su haz."
        actions={
          <div className="flex items-center gap-2">
            <Select value={fState} onValueChange={(v: string | null) => setFState(v ?? "all")}>
              <SelectTrigger className="w-40 text-xs" aria-label="estado">
                <span>{fState === "all" ? "Todos los estados" : fState === "ok" ? "En ruta" : "Varados"}</span>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Todos los estados</SelectItem>
                <SelectItem value="ok">En ruta</SelectItem>
                <SelectItem value="down">Varados</SelectItem>
              </SelectContent>
            </Select>
            <Select value={fTls} onValueChange={(v: string | null) => setFTls(v ?? "all")}>
              <SelectTrigger className="w-32 text-xs" aria-label="tls">
                <span>{fTls === "all" ? "Todos los TLS" : fTls === "tls" ? "Con TLS" : "Sin TLS"}</span>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Todos los TLS</SelectItem>
                <SelectItem value="tls">Con TLS</SelectItem>
                <SelectItem value="off">Sin TLS</SelectItem>
              </SelectContent>
            </Select>
            <Link to="/routes" className="text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline">
              Gestionar →
            </Link>
          </div>
        }
      />
      {!rows && <p className="text-sm text-muted-foreground">cargando…</p>}
      {rows && (
        <>
          <RadialMap links={filteredLinks} selected={selected} onSelect={setSelected} />
          {selected && byHost.get(selected) && (
            <button
              onClick={() => setSelected(null)}
              className="tnum w-fit rounded-full border px-3 py-1 font-mono text-[11px] text-muted-foreground hover:border-ring hover:text-foreground"
            >
              ✕ limpiar filtro: {selected}
            </button>
          )}
          <div className="overflow-hidden rounded-lg border" role="table" aria-label="rutas del proxy">
          {(selected ? rows.filter((r) => r.hosts.includes(selected)) : rows)
            .filter((r) => {
              const l = byHost.get(r.hosts[0]);
              return !l || filteredLinks.includes(l) || r.hosts.some((h) => filteredLinks.some((f) => f.host === h));
            })
            .map((r) => (
            <div
              key={r.hosts.join(",") + r.target}
              className="flex items-center gap-3 border-b bg-card/40 px-4 py-3 last:border-0"
              role="row"
            >
              <Dot status={r.ok ? "ok" : "failed"} />
              <div className="min-w-0 flex-1">
                <p className="truncate font-mono text-xs">{r.hosts.join(", ")}</p>
                <p className="tnum truncate font-mono text-[11px] text-muted-foreground">→ {r.target}</p>
              </div>
              <span className="tnum hidden font-mono text-[11px] text-muted-foreground sm:block">{r.ok ? "backend ok" : "backend caído"}</span>
              {r.tls.map((t, i) => (
                <Badge
                  key={i}
                  variant={t === "off" ? "outline" : "secondary"}
                  className="tnum font-mono text-[11px] font-normal"
                >
                  {t === "off" ? "sin TLS" : `TLS ${t}`}
                </Badge>
              ))}
            </div>
          ))}
          {rows.length === 0 && (
            <EmptyState title="Sin routes" hint="Agrega tu primer dominio en Domains." />
          )}
          </div>
        </>
      )}
    </div>
  );
}
