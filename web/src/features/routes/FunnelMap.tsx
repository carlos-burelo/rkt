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
    <section aria-label="mapa de rutas" className="border border-border bg-card/30">
      <header className="flex items-center gap-2 border-b border-border bg-secondary/20 px-3 py-2">
        <span aria-hidden className="h-1.5 w-1.5 bg-signal" />
        <h3 className="text-[12px] font-medium">Rutas</h3>
        <span className="tnum font-mono text-[9px] text-muted-foreground">{links.length} hosts</span>
        <span className="flex-1" />
        <Link to="/proxy" className="inline-flex items-center gap-1 font-mono text-[9px] text-muted-foreground hover:text-foreground">
          ver todo <ArrowRight className="h-3 w-3" />
        </Link>
      </header>
      <ul className="divide-y divide-border/60">
        {groups.map((g) => (
          <li
            key={g.target}
            className="grid items-center gap-2 px-3 py-2.5 md:grid-cols-[minmax(0,1fr)_2.5rem_auto]"
          >
            <div className="flex min-w-0 flex-wrap gap-x-3 gap-y-1">
              {g.hosts.map((l) => (
                <span
                  key={l.host}
                  className={cn("inline-flex max-w-64 items-center gap-1.5", l.stale ? "opacity-55" : "")}
                  title={`${l.host} → ${l.target}`}
                >
                  <span
                    aria-hidden
                    className={`h-1.5 w-1.5 shrink-0 ${
                      l.stale ? "bg-muted-foreground/35" : l.ok ? "bg-ok" : "bg-bad"
                    }`}
                  />
                  <span className="truncate font-mono text-[11px]">{l.host}</span>
                </span>
              ))}
            </div>
            <svg width="40" height="10" viewBox="0 0 40 10" fill="none" aria-hidden className="hidden shrink-0 text-muted-foreground/35 md:block">
              <path d="M0 5 H40" stroke="currentColor" strokeWidth="1" />
              <path d="M35 1.5 L39.5 5 L35 8.5" stroke="currentColor" strokeWidth="1" fill="none" />
            </svg>
            <span
              className="flex w-fit items-center gap-2 border border-border bg-card/50 px-2 py-1"
              title={`${g.hosts.length} ruta${g.hosts.length > 1 ? "s" : ""} → ${g.target}`}
            >
              <Server className="h-3 w-3 text-muted-foreground" />
              <span className="tnum font-mono text-[10px]">{g.short}</span>
              <span className="font-mono text-[8px] tracking-[0.08em] text-muted-foreground/60 uppercase">
                {g.hosts[0]?.tls === "off" ? "sin tls" : `tls ${g.hosts[0]?.tls ?? "?"}`}
              </span>
            </span>
          </li>
        ))}
      </ul>
      <div className="border-t border-border px-3 py-2">
        <button
          onClick={onAdd}
          className="inline-flex items-center gap-1.5 border border-border px-2 py-1 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:border-input hover:text-foreground"
        >
          <Plus className="h-3 w-3" /> agregar ruta
        </button>
      </div>
    </section>
  );
}
