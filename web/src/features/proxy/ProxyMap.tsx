import { useState } from "react";
import { Monitor, Network } from "lucide-react";
import { cn } from "@/lib/utils";
import { Dot } from "../../shared/components/ui";
import { GraphLink, radialPos } from "./graph";
import { useI18n } from "../../shared/i18n";

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
  const { t, tn } = useI18n();
  const [hover, setHover] = useState<string | null>(null);
  const active = hover ?? selected;
  if (links.length === 0) return null;
  const compact = links.length <= 2;

  return (
    <div className="border border-border bg-ink/35">
      <header className="flex items-center gap-2 border-b border-border bg-secondary/20 px-3 py-1.5">
        <span aria-hidden className="h-1.5 w-1.5 bg-signal" />
        <span className="micro">{t("serve.map.topology")}</span>
        <span className="flex-1" />
        <span className="tnum font-mono text-[9px] text-muted-foreground">
          {tn("serve.hosts.count", links.length)} · {t("serve.map.realDial")}
        </span>
      </header>
      {/* móvil: ledger host → backend. El radial necesita ancho para
          orbitar 6 nodos sin solapes, así que solo se dibuja en sm+. */}
      <ul className="divide-y divide-border sm:hidden">
        {links.map((l) => (
          <li key={l.host}>
            <button
              onClick={() => onSelect(selected === l.host ? null : l.host)}
              className={cn(
                "flex w-full items-center gap-2 px-3 py-2 text-left",
                active === l.host ? "bg-secondary/40" : "hover:bg-accent/40",
              )}
            >
              <Dot status={l.ok ? "ok" : "failed"} />
              <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{l.host}</span>
              <span aria-hidden className="shrink-0 text-[9px] text-muted-foreground">→</span>
              <span className="tnum shrink-0 font-mono text-[9px] text-muted-foreground">{l.short}</span>
            </button>
          </li>
        ))}
      </ul>
      <div className="grid-paper relative hidden sm:block">
        <div
          className={`relative mx-auto w-full ${compact ? "aspect-[16/8] max-w-xl" : "aspect-[8/5] max-w-3xl"}`}
          role="img"
          aria-label={tn("serve.map.aria", links.length)}
        >
          <svg className="absolute inset-0 h-full w-full" viewBox="0 0 800 500" aria-hidden>
            <ellipse cx="400" cy="250" rx="272" ry="168" fill="none" stroke="currentColor" strokeOpacity="0.14" strokeDasharray="3 7" />
            <ellipse cx="400" cy="250" rx="168" ry="104" fill="none" stroke="currentColor" strokeOpacity="0.1" strokeDasharray="2 8" />
            <line x1="400" y1="250" x2="400" y2="30" stroke="currentColor" strokeOpacity="0.12" strokeDasharray="4 6" />
            {links.map((l) => {
              const p = radialPos(links.indexOf(l), links.length);
              const x = p.x * 800;
              const y = p.y * 500;
              const isActive = active === l.host;
              const dim = active !== null && active !== undefined && !isActive;
              return (
                <line
                  key={l.host}
                  x1="400"
                  y1="250"
                  x2={x}
                  y2={y}
                  stroke={l.ok ? "var(--ok)" : "var(--bad)"}
                  strokeOpacity={isActive ? 0.9 : dim ? 0.12 : 0.38}
                  strokeWidth={isActive ? 2 : 1.25}
                  strokeDasharray="6 5"
                  style={{ transition: "stroke-opacity 180ms ease-out" }}
                />
              );
            })}
          </svg>

          {/* nodo centro: edge */}
          <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 text-center">
            <div className="mx-auto flex h-12 w-12 items-center justify-center border border-signal/50 bg-card">
              <Network className="h-5 w-5 text-signal" strokeWidth={1.8} />
            </div>
            <p className="mt-1.5 font-mono text-[10px] font-medium">rkt edge</p>
            <p className="tnum font-mono text-[9px] text-muted-foreground">:80 / :443</p>
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
                  "absolute w-28 -translate-x-1/2 -translate-y-1/2 border bg-card/95 px-2 py-1.5 text-left sm:w-36",
                  "hover:border-input focus-visible:border-signal",
                  isActive ? "border-signal" : "border-border",
                  dim && "opacity-40",
                )}
                style={{
                  left: `${p.x * 100}%`,
                  top: `${p.y * 100}%`,
                  transition: "opacity 180ms ease-out, border-color 180ms ease-out",
                }}
                title={`${l.host} → ${l.target} · ${
                  l.ok ? t("serve.proxy.backendOk") : t("serve.proxy.backendDown")
                } · ${l.tls === "off" ? t("serve.tls.noneUpper") : t("serve.tls.valueUpper", { version: l.tls })}`}
              >
                <span className="flex items-center gap-1.5">
                  <Dot status={l.ok ? "ok" : "failed"} />
                  <span className="truncate font-mono text-[10px]">{l.host}</span>
                </span>
                <span className="tnum mt-0.5 block truncate font-mono text-[9px] text-muted-foreground">
                  → {l.short}
                </span>
              </button>
            );
          })}

          {/* backend inspector: aparece al seleccionar */}
          {selected &&
            (() => {
              const l = links.find((x) => x.host === selected);
              if (!l) return null;
              return (
                <div className="absolute right-2 bottom-2 flex items-center gap-2 border border-border bg-card/95 px-2.5 py-1.5">
                  <span className="flex h-7 w-7 items-center justify-center border border-border bg-secondary/60">
                    <Monitor className="h-3.5 w-3.5 text-muted-foreground" />
                  </span>
                  <span>
                    <span className="block font-mono text-[10px]">{l.short}</span>
                    <span className="tnum block font-mono text-[9px] text-muted-foreground">
                      {l.ok ? t("serve.proxy.backendOk") : t("serve.proxy.backendDown")} ·{" "}
                      {l.tls === "off" ? t("serve.tls.noneUpper") : t("serve.tls.valueUpper", { version: l.tls })}
                    </span>
                  </span>
                </div>
              );
            })()}
        </div>
      </div>
    </div>
  );
}
