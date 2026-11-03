import { useEffect, useState } from "react";
import { fetchProxy, ProxyRoute } from "./api";
import { Dot } from "../../shared/components/ui";
import { Badge } from "@/components/ui/badge";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
import { useUI } from "../../app/ui";

// Proxy: lo que Vercel esconde. Cada host, a dónde va, si el backend
// responde (dial) y estado del cert. Solo lectura.
export function Proxy() {
  const fail = useUI((u) => u.fail);
  const [rows, setRows] = useState<ProxyRoute[] | null>(null);

  useEffect(() => {
    fetchProxy().then(setRows).catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="space-y-3">
      <PageHeader
        title="Proxy"
        description="Alcance real por ruta (dial TCP al backend + cert ACME). Vercel no muestra esto."
      />
      {!rows && <p className="text-sm text-muted-foreground">cargando…</p>}
      {rows && (
        <div className="overflow-hidden rounded-lg border" role="table" aria-label="rutas del proxy">
          {rows.map((r) => (
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
      )}
    </div>
  );
}
