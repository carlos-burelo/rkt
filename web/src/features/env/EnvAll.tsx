import { useEffect, useState } from "react";
import { Lock } from "lucide-react";
import { useNavigate } from "react-router";
import { listApps, getEnv } from "../apps/api";
import { AppMeta } from "../../shared/lib/types";
import { Input } from "@/components/ui/input";
import { Avatar } from "../../shared/components/ui";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
import { useUI } from "../../app/ui";

// EnvAll vista agrupada por app con buscador global.
// Valores ocultos (detalle en cada app); aquí solo inventario.
export function EnvAll() {
  const fail = useUI((u) => u.fail);
  const navigate = useNavigate();
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
          <div key={app} className="overflow-hidden rounded-lg border">
            <button
              onClick={() => navigate(`/app/${app}`)}
              className="flex w-full items-center gap-3 border-b bg-card px-4 py-3 text-left hover:bg-accent/40"
            >
              <Avatar name={app} />
              <span className="min-w-0 flex-1 truncate font-mono text-xs">{app}</span>
              <span className="tnum font-mono text-[11px] text-muted-foreground">
                {Object.keys(vars).length} vars
              </span>
            </button>
            {keys.map((k) => (
              <div key={k} className="flex items-center gap-3 border-b bg-card/40 px-4 py-2 last:border-0">
                <Lock className="h-3 w-3 shrink-0 text-muted-foreground" aria-label="secreto" />
                <span className="w-56 truncate font-mono text-xs">{k}</span>
                <span className="tnum font-mono text-[11px] text-muted-foreground">secreto · editar en app</span>
              </div>
            ))}
            {keys.length === 0 && <p className="bg-card/40 px-4 py-3 text-xs text-muted-foreground">Sin vars.</p>}
          </div>
        );
      })}
      {rows.length === 0 && <EmptyState title="Sin apps" hint="Registra tu primera app." />}
    </div>
  );
}
