import { useEffect, useState } from "react";
import { fetchProxy, ProxyRoute } from "./api";
import { Dot } from "../../shared/components/ui";

// Proxy: lo que Vercel esconde. Cada host, a dónde va, si el backend
// responde (dial) y estado del cert. Solo lectura.
export function Proxy({ fail }: { fail: (e: unknown) => void }) {
  const [rows, setRows] = useState<ProxyRoute[] | null>(null);

  useEffect(() => {
    fetchProxy().then(setRows).catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="space-y-3">
      <p className="text-xs text-zinc-500">
        Alcance real por ruta (dial TCP al backend + cert ACME). Vercel no muestra esto.
      </p>
      {!rows && <p className="text-sm text-zinc-500">cargando…</p>}
      {rows && (
        <div className="overflow-hidden rounded-lg border border-zinc-800">
          {rows.map((r) => (
            <div
              key={r.hosts.join(",") + r.target}
              className="flex items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-3 last:border-0"
            >
              <Dot status={r.ok ? "ok" : "failed"} />
              <div className="min-w-0">
                <p className="font-mono text-xs">{r.hosts.join(", ")}</p>
                <p className="truncate font-mono text-[11px] text-zinc-500">→ {r.target}</p>
              </div>
              <span className="flex-1" />
              <span className="font-mono text-[11px] text-zinc-500">{r.ok ? "backend ok" : "backend caído"}</span>
              {r.tls.map((t, i) => (
                <span
                  key={i}
                  className={`rounded-full px-2 py-0.5 font-mono text-[11px] ${
                    t === "off" ? "bg-zinc-800 text-zinc-500" : "bg-emerald-500/15 text-emerald-300"
                  }`}
                >
                  {t === "off" ? "sin TLS" : `TLS ${t}`}
                </span>
              ))}
            </div>
          ))}
          {rows.length === 0 && <p className="p-6 text-sm text-zinc-500">Sin routes.</p>}
        </div>
      )}
    </div>
  );
}
