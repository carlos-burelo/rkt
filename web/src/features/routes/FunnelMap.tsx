import { Link } from "react-router";
import { ArrowRight, Plus, Server } from "lucide-react";
import { cn } from "@/lib/utils";
import { GraphLink, groupByTarget } from "../proxy/graph";
import { useI18n } from "../../shared/i18n";

const ROW = 34;
const GAP = 10;

// Flowchart host → backend: columna de hosts, conectores bezier y caja
// de destino centrada. Geometría fija (sin medir DOM) para no romperse.
export function FunnelMap({
  links,
  onAdd,
}: {
  links: GraphLink[];
  onAdd?: () => void;
}) {
  const { t, tn } = useI18n();
  const groups = groupByTarget(links);
  if (links.length === 0) return null;

  return (
    <section aria-label={t("serve.funnel.aria")} className="border border-border bg-card/30">
      <header className="flex flex-wrap items-center gap-2 border-b border-border bg-secondary/20 px-3 py-2">
        <span aria-hidden className="h-1.5 w-1.5 bg-signal" />
        <h3 className="text-[12px] font-medium">{t("serve.funnel.title")}</h3>
        <span className="tnum font-mono text-[9px] text-muted-foreground">
          {tn("serve.hosts.count", links.length)} · {tn("serve.funnel.destinations", groups.length)}
        </span>
        <span className="flex-1" />
        <Link to="/routes?tab=diagnostico" className="inline-flex items-center gap-1 font-mono text-[9px] text-muted-foreground hover:text-foreground">
          {t("serve.funnel.viewTopology")} <ArrowRight className="h-3 w-3" />
        </Link>
      </header>

      <ul className="divide-y divide-border/60">
        {groups.map((g) => {
          const n = g.hosts.length;
          const height = n * ROW + (n - 1) * GAP;
          const mid = height / 2;
          return (
            <li key={g.target} className="px-3 py-3">
              {/* desktop: flowchart con conectores */}
              <div className="hidden items-stretch gap-2 sm:flex">
                <div className="flex w-56 max-w-[45%] shrink-0 flex-col" style={{ gap: GAP }}>
                  {g.hosts.map((l) => (
                    <span
                      key={l.host}
                      className="flex items-center gap-2 border border-border bg-card/50 px-2"
                      style={{ height: ROW }}
                      title={`${l.host} → ${l.target}`}
                    >
                      <span
                        aria-hidden
                        className={cn(
                          "h-1.5 w-1.5 shrink-0",
                          l.stale ? "bg-muted-foreground/40" : l.ok ? "bg-ok" : "bg-bad",
                        )}
                      />
                      <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{l.host}</span>
                    </span>
                  ))}
                </div>

                <svg
                  className="shrink-0 text-muted-foreground/45"
                  width={76}
                  height={height}
                  viewBox={`0 0 100 ${height}`}
                  preserveAspectRatio="none"
                  aria-hidden
                >
                  {g.hosts.map((l, i) => {
                    const y = i * (ROW + GAP) + ROW / 2;
                    return (
                      <path
                        key={l.host}
                        d={`M0 ${y} C 45 ${y}, 55 ${mid}, 96 ${mid}`}
                        fill="none"
                        stroke="currentColor"
                        strokeWidth={1.2}
                        strokeDasharray="5 4"
                        vectorEffect="non-scaling-stroke"
                      />
                    );
                  })}
                  <path
                    d={`M92 ${mid - 4} L97 ${mid} L92 ${mid + 4}`}
                    fill="none"
                    stroke="currentColor"
                    strokeWidth={1.2}
                    vectorEffect="non-scaling-stroke"
                  />
                </svg>

                <div className="flex min-w-0 flex-1 items-center">
                  <span
                    className="flex min-w-0 items-center gap-2 border border-border bg-card/60 px-2 py-1.5"
                    title={`${tn("serve.routes.count", g.hosts.length)} → ${g.target}`}
                  >
                    <Server className="h-3 w-3 shrink-0 text-muted-foreground" />
                    <span className="tnum truncate font-mono text-[10px]">{g.short}</span>
                    <span className="shrink-0 font-mono text-[8px] tracking-[0.08em] text-muted-foreground/60 uppercase">
                      {g.hosts[0]?.tls === "off"
                        ? t("serve.tls.none")
                        : t("serve.tls.value", { version: g.hosts[0]?.tls ?? "?" })}
                    </span>
                  </span>
                </div>
              </div>

              {/* móvil: mismo flujo, apilado (sin conectores que se corten) */}
              <div className="space-y-1.5 sm:hidden">
                {g.hosts.map((l) => (
                  <div key={l.host} className="flex items-center gap-2 border border-border bg-card/50 px-2 py-1.5">
                    <span
                      aria-hidden
                      className={cn("h-1.5 w-1.5 shrink-0", l.stale ? "bg-muted-foreground/40" : l.ok ? "bg-ok" : "bg-bad")}
                    />
                    <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{l.host}</span>
                    <span aria-hidden className="text-[9px] text-muted-foreground">→</span>
                    <span className="tnum shrink-0 font-mono text-[9px] text-muted-foreground">{g.short}</span>
                  </div>
                ))}
              </div>
            </li>
          );
        })}
      </ul>

      <div className="border-t border-border px-3 py-2">
        <button
          onClick={onAdd}
          className="inline-flex items-center gap-1.5 border border-border px-2 py-1 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:border-input hover:text-foreground"
        >
          <Plus className="h-3 w-3" /> {t("serve.funnel.addRoute")}
        </button>
      </div>
    </section>
  );
}
