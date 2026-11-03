import { useState } from "react";
import { Link } from "react-router";
import { Plus, Server } from "lucide-react";
import { cn } from "@/lib/utils";
import { GraphLink, groupByTarget } from "../proxy/graph";

// Funnel compacto host → backend (imagen 2) con vocabulario terminal:
// hairlines, sin azul Vercel, fondo punteado. Click resalta el haz.
export function FunnelMap({
  links,
  onAdd,
}: {
  links: GraphLink[];
  onAdd?: () => void;
}) {
  const [active, setActive] = useState<string | null>(null);
  const groups = groupByTarget(links);
  if (links.length === 0) return null;
  const leftCount = links.length;
  const yLeft = (i: number) => ((i + 0.5) / leftCount) * 100;

  // índice vertical del grupo destino dentro del tramo que le toca
  let acc = 0;
  const groupY = new Map<string, number>();
  for (const g of groups) {
    const start = (acc / leftCount) * 100;
    const end = ((acc + g.hosts.length) / leftCount) * 100;
    groupY.set(g.target, (start + end) / 2);
    acc += g.hosts.length;
  }

  return (
    <section aria-label="mapa de rutas" className="overflow-hidden rounded-xl border">
      <div className="flex items-center gap-2 border-b px-4 py-2.5">
        <h3 className="text-sm font-medium">Rutas</h3>
        <span className="tnum rounded border px-1.5 font-mono text-[10px] text-muted-foreground">
          {links.length}
        </span>
        <span className="flex-1" />
        <Link to="/proxy" className="text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline">
          Ver todo →
        </Link>
      </div>
      <div
        className="relative bg-card/40"
        style={{
          backgroundImage: "radial-gradient(circle, oklch(1 0 0 / 10%) 1px, transparent 1.2px)",
          backgroundSize: "13px 13px",
        }}
      >
        <svg className="pointer-events-none absolute inset-0 h-full w-full" viewBox="0 0 100 100"
          preserveAspectRatio="none" aria-hidden>
          {links.map((l, i) => {
            const y2 = groupY.get(l.target) ?? 50;
            const isActive = active === l.host || active === l.target;
            const dim = active !== null && !isActive;
            const stroke = l.stale ? "currentColor" : l.ok ? "#34d399" : "#f87171";
            return (
              <path
                key={l.host}
                d={`M 40 ${yLeft(i)} C 50 ${yLeft(i)}, 56 ${y2}, 68 ${y2}`}
                fill="none"
                stroke={stroke}
                strokeOpacity={l.stale ? (dim ? 0.08 : 0.25) : isActive ? 0.95 : dim ? 0.12 : 0.4}
                strokeWidth={isActive ? 1.1 : 0.5}
                vectorEffect="non-scaling-stroke"
                style={{ transition: "stroke-opacity 180ms ease-out" }}
              />
            );
          })}
        </svg>
        <div className="relative grid grid-cols-[1fr_auto_1fr] items-stretch gap-2 p-4">
          <ul className="space-y-2">
            {links.map((l) => (
              <li key={l.host}>
                <button
                  onClick={() => setActive(active === l.host ? null : l.host)}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-full border bg-background/90 py-1.5 pl-3 pr-2 text-left",
                    "hover:border-ring",
                    active === l.host ? "border-ring" : "border-border",
                    active !== null && active !== l.host && active !== l.target && "opacity-50",
                  )}
                  style={{ transition: "opacity 180ms ease-out, border-color 180ms ease-out" }}
                  title={`${l.host} → ${l.target}`}
                >
                  <span className="min-w-0 flex-1 truncate font-mono text-xs text-sky-300 underline decoration-sky-300/40 underline-offset-2">
                    {l.host}
                  </span>
                  <span className="hidden shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] text-muted-foreground sm:flex">
                    <Server className="h-3 w-3" />
                    {l.tls === "off" ? "sin TLS" : "publicada"}
                  </span>
                </button>
              </li>
            ))}
            <li>
              <button
                onClick={onAdd}
                className="flex w-fit items-center gap-1.5 rounded-full border border-dashed px-3 py-1.5 text-xs text-muted-foreground hover:border-ring hover:text-foreground"
              >
                <Plus className="h-3.5 w-3.5" /> Agregar ruta
              </button>
            </li>
          </ul>
          <div aria-hidden className="w-2" />
          <ul className="space-y-2">
            {groups.map((g) => (
              <li key={g.target} style={{ marginTop: 0 }}>
                <button
                  onClick={() => setActive(active === g.target ? null : g.target)}
                  className={cn(
                    "flex items-center gap-2 rounded-full border bg-background/90 px-3 py-1.5",
                    "hover:border-ring",
                    active === g.target ? "border-ring" : "border-border",
                    active !== null && active !== g.target && !g.hosts.some((h) => h.host === active) && "opacity-50",
                  )}
                  style={{ transition: "opacity 180ms ease-out, border-color 180ms ease-out" }}
                  title={`${g.hosts.length} ruta${g.hosts.length > 1 ? "s" : ""} → ${g.target}`}
                >
                  <Server className="h-3.5 w-3.5 text-muted-foreground" />
                  <span className="tnum font-mono text-xs">{g.short}</span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  );
}
