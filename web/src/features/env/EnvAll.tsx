import { useEffect, useState } from "react";
import { listApps, getEnv } from "../apps/api";
import { AppMeta } from "../../shared/lib/types";
import { Input } from "@/components/ui/input";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
import { useUI } from "../../app/ui";

// EnvAll vista agrupada por app con buscador global.
// Valores ocultos (detalle en cada app); aquí solo inventario.
export function EnvAll() {
  const fail = useUI((u) => u.fail);
  const [q, setQ] = useState("");
  const [rows, setRows] = useState<{ app: string; vars: Record<string, string> }[]>([]);

  useEffect(() => {
    listApps()
      .then(async (apps: AppMeta[]) => {
        const out: { app: string; vars: Record<string, string> }[] = [];
        for (const a of apps) {
          try {
            out.push({ app: a.name, vars: await getEnv(a.name) });
          } catch (e) {
            fail(e);
            return;
          }
        }
        setRows(out);
      })
      .catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const f = q.toLowerCase();
  return (
    <div className="space-y-3">
      <PageHeader title="Environment" description="Inventario por app (valores ocultos; edición en cada app)." />
      <Input
        value={q}
        onChange={(e) => setQ(e.target.value)}
        placeholder="Buscar variable o app…"
        className="max-w-md text-xs"
      />
      {rows.map(({ app, vars }) => {
        const keys = Object.keys(vars).filter(
          (k) => f === "" || k.toLowerCase().includes(f) || app.toLowerCase().includes(f),
        );
        if (f !== "" && keys.length === 0) return null;
        return (
          <div key={app} className="rounded-lg border bg-card p-4">
            <p className="mb-2 font-mono text-xs">
              {app} <span className="text-muted-foreground">{Object.keys(vars).length}</span>
            </p>
            {keys.map((k) => (
              <p key={k} className="flex gap-3 py-0.5 font-mono text-xs">
                <span className="w-56 truncate">{k}</span>
                <span className="text-muted-foreground">🔒 secreto</span>
              </p>
            ))}
            {keys.length === 0 && <p className="text-xs text-muted-foreground">Sin vars.</p>}
          </div>
        );
      })}
      {rows.length === 0 && <EmptyState title="Sin apps" hint="Registra tu primera app." />}
    </div>
  );
}
