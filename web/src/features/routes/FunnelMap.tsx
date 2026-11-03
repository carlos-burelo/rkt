import { Link } from "react-router";
import { ArrowRight, Plus, Server } from "lucide-react";
import { cn } from "@/lib/utils";
import { GraphLink, groupByTarget } from "../proxy/graph";

// Funnel host → backend en filas: cada fila su conector, sin solapes.
// Sin azul Vercel; verde/rojo solo por estado real del dial.
export function FunnelMap({
  links,
  onAdd,
}: {
  links: GraphLink[];
  onAdd?: () => void;
}) {
  const groups = groupByTarget(links);
  if (links.length === 0) return null;

  return (
    <section aria-label="mapa de rutas" className="overflow-hidden rounded-xl border">
      <div className="flex items-center gap-2 border-b px-4 py-2.5">
        <h3 className="text-sm font-medium">Rutas</h3>
        <span className="tnum rounded border px-1.5 font-mono text-[10px] text-muted-foreground">
          {links.length}
        </span>
        <span className="flex-1" />
        <Link to="/proxy" className="inline-flex items-center gap-1 text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline">
          Ver todo <ArrowRight className="h-3 w-3" />
        </Link>
      </div>
      <ul className="divide-y divide-border/60">
        {groups.map((g) => (
          <li key={g.target} className="flex items-center gap-2 px-4 py-2.5">
            <div className="flex min-w-0 flex-1 flex-wrap gap-1.5">
              {g.hosts.map((l) => (
                <span
                  key={l.host}
                  className={cn(
                    "inline-flex max-w-56 items-center gap-1.5 rounded-full border bg-background px-2.5 py-1",
                    l.stale ? "opacity-60" : "",
                  )}
                  title={`${l.host} → ${l.target}`}
                >
                  <span
                    className={`h-1.5 w-1.5 shrink-0 rounded-full ${l.stale ? "bg-zinc-500" : l.ok ? "bg-emerald-400" : "bg-red-400"}`}
                  />
                  <span className="truncate font-mono text-xs">{l.host}</span>
                </span>
              ))}
            </div>
            <svg width="48" height="12" viewBox="0 0 48 12" fill="none" aria-hidden className="shrink-0">
              <path
                d="M1 6 H47"
                stroke="currentColor"
                strokeOpacity="0.3"
                strokeWidth="1"
              />
              <path d="M41 2 L47 6 L41 10" stroke="currentColor" strokeOpacity="0.3" strokeWidth="1" fill="none" />
            </svg>
            <span
              className="inline-flex shrink-0 items-center gap-1.5 rounded-full border bg-background px-2.5 py-1"
              title={`${g.hosts.length} ruta${g.hosts.length > 1 ? "s" : ""} → ${g.target}`}
            >
              <Server className="h-3 w-3 text-muted-foreground" />
              <span className="tnum font-mono text-xs">{g.short}</span>
            </span>
          </li>
        ))}
      </ul>
      <div className="border-t px-4 py-2.5">
        <button
          onClick={onAdd}
          className="inline-flex items-center gap-1.5 rounded-full border border-dashed px-3 py-1.5 text-xs text-muted-foreground hover:border-ring hover:text-foreground"
        >
          <Plus className="h-3.5 w-3.5" /> Agregar ruta
        </button>
      </div>
    </section>
  );
}
