import { useState } from "react";
import { Monitor, Cloud } from "lucide-react";
import { cn } from "@/lib/utils";
import { Dot } from "../../shared/components/ui";
import { GraphLink, radialPos } from "./graph";

// Mapa radial: edge al centro, hosts en órbita. SVG dibuja órbitas +
// enlaces; nodos son <button> HTML (foco teclado, sin foreignObject).
export function RadialMap({
  links,
  selected,
  onSelect,
}: {
  links: GraphLink[];
  selected: string | null;
  onSelect: (host: string | null) => void;
}) {
  const [hover, setHover] = useState<string | null>(null);
  const active = hover ?? selected;
  if (links.length === 0) return null;

  return (
    <div
      className="relative overflow-hidden rounded-xl border bg-background"
      style={{
        backgroundImage: "radial-gradient(circle, oklch(1 0 0 / 10%) 1px, transparent 1.4px)",
        backgroundSize: "16px 16px",
      }}
    >
      <div className="relative mx-auto aspect-[8/5] w-full max-w-3xl" role="img"
        aria-label={`topología proxy: edge con ${links.length} ruta${links.length > 1 ? "s" : ""}`}>
        <svg className="absolute inset-0 h-full w-full" viewBox="0 0 800 500" aria-hidden>
          <ellipse cx="400" cy="250" rx="272" ry="168" fill="none"
            stroke="currentColor" strokeOpacity="0.14" strokeDasharray="3 7" />
          <ellipse cx="400" cy="250" rx="168" ry="104" fill="none"
            stroke="currentColor" strokeOpacity="0.1" strokeDasharray="2 8" />
          <line x1="400" y1="250" x2="400" y2="30" stroke="currentColor"
            strokeOpacity="0.12" strokeDasharray="4 6" />
          {links.map((l) => {
            const p = radialPos(links.indexOf(l), links.length);
            const x = p.x * 800;
            const y = p.y * 500;
            const isActive = active === l.host;
            const dim = active !== null && active !== undefined && !isActive;
            return (
              <line
                key={l.host}
                x1="400" y1="250" x2={x} y2={y}
                stroke={l.ok ? "#34d399" : "#f87171"}
                strokeOpacity={isActive ? 0.9 : dim ? 0.15 : 0.45}
                strokeWidth={isActive ? 2 : 1.25}
                strokeDasharray="6 5"
                style={{ transition: "stroke-opacity 180ms ease-out" }}
              />
            );
          })}
        </svg>

        {/* nodo centro: edge */}
        <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 text-center">
          <div
            className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-signal text-[oklch(0.16_0.02_40)]"
            style={{ boxShadow: "0 10px 34px -10px oklch(0.646 0.19 42.5 / 55%)" }}
          >
            <Cloud className="h-6 w-6" strokeWidth={2.2} />
          </div>
          <p className="mt-1.5 text-xs font-semibold">rkt edge</p>
          <p className="tnum font-mono text-[10px] text-muted-foreground">:80 / :443</p>
        </div>

        {/* satélites */}
        {links.map((l) => {
          const p = radialPos(links.indexOf(l), links.length);
          const isActive = active === l.host;
          const dim = active !== null && active !== undefined && !isActive;
          return (
            <button
              key={l.host}
              onClick={() => onSelect(selected === l.host ? null : l.host)}
              onMouseEnter={() => setHover(l.host)}
              onMouseLeave={() => setHover(null)}
              onFocus={() => setHover(l.host)}
              onBlur={() => setHover(null)}
              className={cn(
                "absolute w-28 -translate-x-1/2 -translate-y-1/2 rounded-lg border bg-card/95 px-2.5 py-2 text-left sm:w-36",
                "hover:border-ring focus-visible:border-ring",
                isActive ? "border-ring" : "border-border",
                dim && "opacity-45",
              )}
              style={{
                left: `${p.x * 100}%`,
                top: `${p.y * 100}%`,
                transition: "opacity 180ms ease-out, border-color 180ms ease-out",
              }}
              title={`${l.host} → ${l.target} · ${l.ok ? "backend ok" : "backend caído"} · ${l.tls === "off" ? "sin TLS" : `TLS ${l.tls}`}`}
            >
              <span className="flex items-center gap-1.5">
                <Dot status={l.ok ? "ok" : "failed"} />
                <span className="truncate font-mono text-xs">{l.host}</span>
              </span>
              <span className="tnum mt-0.5 block truncate font-mono text-[10px] text-muted-foreground">
                → {l.short}
              </span>
            </button>
          );
        })}

        {/* backend инспектор: aparece al seleccionar (como imagen 1: x + ip) */}
        {selected && (() => {
          const l = links.find((x) => x.host === selected);
          if (!l) return null;
          return (
            <div className="absolute bottom-3 right-3 flex items-center gap-2 rounded-lg border bg-card/95 px-3 py-2">
              <span className="flex h-8 w-8 items-center justify-center rounded-full bg-secondary">
                <Monitor className="h-4 w-4 text-violet-300" />
              </span>
              <span>
                <span className="block font-mono text-xs">{l.short}</span>
                <span className="tnum block font-mono text-[10px] text-muted-foreground">
                  {l.ok ? "backend ok" : "backend caído"} · {l.tls === "off" ? "sin TLS" : `TLS ${l.tls}`}
                </span>
              </span>
            </div>
          );
        })()}
      </div>
    </div>
  );
}
