import { useEffect, useState } from "react";
import { fetchProxy, ProxyRoute } from "./api";
import { Dot } from "../../shared/components/ui";
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
        <div className="overflow-hidden rounded-lg border">
          {rows.map((r) => (
            <div
              key={r.hosts.join(",") + r.target}
              className="flex items-center gap-3 border-b bg-card/40 px-4 py-3 last:border-0"
            >
              <Dot status={r.ok ? "ok" : "failed"} />
              <div className="min-w-0">
                <p className="font-mono text-xs">{r.hosts.join(", ")}</p>
                <p className="truncate font-mono text-[11px] text-muted-foreground">→ {r.target}</p>
              </div>
              <span className="flex-1" />
              <span className="font-mono text-[11px] text-muted-foreground">{r.ok ? "backend ok" : "backend caído"}</span>
              {r.tls.map((t, i) => (
                <span
                  key={i}
                  className={`rounded-full px-2 py-0.5 font-mono text-[11px] ${
                    t === "off" ? "bg-secondary text-muted-foreground" : "bg-emerald-500/15 text-emerald-300"
                  }`}
                >
                  {t === "off" ? "sin TLS" : `TLS ${t}`}
                </span>
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
