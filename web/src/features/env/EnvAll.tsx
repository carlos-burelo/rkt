import { useEffect, useState } from "react";
import { ArrowRight, Lock, Search } from "lucide-react";
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
  const total = rows.reduce((n, r) => n + Object.keys(r.vars).length, 0);
  const visible = rows.filter(({ app, vars }) => {
    if (f === "") return true;
    return app.toLowerCase().includes(f) || Object.keys(vars).some((k) => k.toLowerCase().includes(f));
  });

  return (
    <div className="space-y-4">
      <PageHeader
        title="Environment"
        description="Inventario por app. Los valores viven enmascarados; la edición ocurre en la ficha de cada unidad."
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {rows.length} apps · {total} vars
          </span>
        }
      />
      <div className="flex items-center gap-2 border border-border bg-card/35 p-2">
        <div className="relative min-w-0 flex-1 sm:max-w-md">
          <Search className="absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="buscar variable o app…"
            className="h-7 border-transparent bg-transparent pl-8 font-mono text-[11px] hover:border-input"
          />
        </div>
        <span className="flex-1" />
        <span className="folio">valores ocultos</span>
      </div>

      {visible.map(({ app, vars }) => {
        const keys = Object.keys(vars).filter((k) => f === "" || k.toLowerCase().includes(f) || app.toLowerCase().includes(f));
        return (
          <section key={app} className="border border-border">
            <button
              onClick={() => navigate(`/app/${app}`)}
              className="flex w-full items-center gap-2.5 border-b border-border bg-secondary/20 px-3 py-2 text-left hover:bg-accent/40"
            >
              <Avatar name={app} />
              <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{app}</span>
              <span className="tnum font-mono text-[9px] text-muted-foreground">{Object.keys(vars).length} vars</span>
              <ArrowRight className="h-3 w-3 text-muted-foreground/40" />
            </button>
            {keys.map((k) => (
              <div key={k} className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-3 py-1.5 last:border-0">
                <Lock className="h-3 w-3 shrink-0 text-muted-foreground/60" aria-label="secreto" />
                <span className="w-52 shrink-0 truncate font-mono text-[11px]">{k}</span>
                <span className="flex-1" />
                <span className="font-mono text-[9px] text-muted-foreground/60">secreto · editar en app</span>
              </div>
            ))}
            {keys.length === 0 && (
              <p className="bg-card/30 px-3 py-3 font-mono text-[10px] text-muted-foreground">sin vars para este filtro</p>
            )}
          </section>
        );
      })}
      {rows.length === 0 && <EmptyState title="Sin apps" hint="Registra tu primera app para ver su inventario." />}
      {rows.length > 0 && visible.length === 0 && (
        <EmptyState title="Sin coincidencias" hint={`Ninguna variable o app coincide con “${q}”.`} />
      )}
    </div>
  );
}
