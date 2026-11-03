import { useState } from "react";
import { useNavigate } from "react-router";
import { ArrowRight, Search } from "lucide-react";
import { dur, rel } from "../../shared/lib/time";
import { Dot, StateRail } from "../../shared/components/ui";
import { CopyButton } from "../../shared/components/feedback";
import { Input } from "@/components/ui/input";
import { useUI } from "../../app/ui";
import { useFleet } from "../overview/hooks";
import { PageHeader, Segmented } from "../../shared/components/patterns";

export type DepStatus = "all" | "ok" | "running" | "failed";

// Deployments: tablero de salidas. Columnas fijas (estado · salida ·
// revisión · duración · hora), filas ordenadas por tiempo, la fila en
// curso late. Duraciones tabulares alineadas a la derecha.
export function Deployments({ bare = false, status, max }: { bare?: boolean; status?: DepStatus; max?: number }) {
  const { deps } = useFleet();
  const appFilter = useUI((u) => u.appFilter);
  const setSel = useUI((u) => u.setSel);
  const navigate = useNavigate();
  const [st, setSt] = useState<DepStatus>(status ?? "all");
  const [q, setQ] = useState("");
  const f = q.trim().toLowerCase();

  const base = deps.filter(
    (d) =>
      (appFilter === "" || d.app === appFilter) &&
      (f === "" || d.app.toLowerCase().includes(f) || d.rev.toLowerCase().includes(f)),
  );
  let vis = base.filter((d) => st === "all" || d.status === st);
  if (max !== undefined) vis = vis.slice(0, max);
  const counts = {
    all: base.length,
    ok: base.filter((d) => d.status === "ok").length,
    running: base.filter((d) => d.status === "running").length,
    failed: base.filter((d) => d.status === "failed").length,
  };

  return (
    <div className="space-y-4">
      {!bare && (
        <PageHeader
          title="Despliegues"
          description="Libro de salidas. La fila en curso late; la revisión y la duración quedan a la derecha. Click abre el manifiesto de la compilación."
          meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{base.length} registros</span>}
        />
      )}
      {!bare && (
        <div className="flex flex-wrap items-center gap-2 border border-border bg-card/35 p-2">
          <div className="relative min-w-52 flex-1">
            <Search className="absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="filtrar por app o rev…"
              className="h-7 border-transparent bg-transparent pl-8 font-mono text-[11px] hover:border-input"
              aria-label="filtrar despliegues"
            />
          </div>
          <Segmented
            value={st}
            onChange={setSt}
            label="filtrar por estado"
            items={[
              { value: "all", label: "todos", count: counts.all },
              { value: "ok", label: "ok", count: counts.ok },
              { value: "running", label: "en curso", count: counts.running },
              { value: "failed", label: "fallidos", count: counts.failed },
            ]}
          />
        </div>
      )}
      <div className="border border-border" role="table" aria-label="tablero de despliegues">
        <div
          className="hidden grid-cols-[1.5rem_minmax(0,1fr)_8rem_4.5rem_5.5rem_1.5rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 md:grid"
          aria-hidden
        >
          <span className="micro">est</span>
          <span className="micro">salida</span>
          <span className="micro">rev</span>
          <span className="micro text-right">dur</span>
          <span className="micro text-right">hora</span>
          <span />
        </div>
        {vis.map((d) => (
          <div
            key={d.id}
            onClick={() => setSel(d)}
            className={`group relative grid cursor-pointer grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-3 border-b border-border/70 bg-card/35 px-3 py-2.5 text-left last:border-0 hover:bg-accent/40 md:grid-cols-[1.5rem_minmax(0,1fr)_8rem_4.5rem_5.5rem_1.5rem] ${
              d.status === "running" ? "animate-row-arrive" : ""
            }`}
            role="row"
          >
            <StateRail status={d.status} />
            <Dot status={d.status} />
            <div className="min-w-0">
              <p className="truncate font-mono text-[12px]">
                {d.app} <span className="tnum text-muted-foreground">#{d.id}</span>
              </p>
              <p className="tnum mt-0.5 truncate font-mono text-[9px] text-muted-foreground md:hidden">
                {d.rev.slice(0, 8)} · {dur(d.started_at, d.finished_at)} ·{" "}
                {d.status === "running" ? "en curso…" : rel(d.finished_at || d.started_at)}
              </p>
              <p className="mt-0.5 hidden truncate font-mono text-[9px] text-muted-foreground md:block">
                {d.triggered_by ? `por ${d.triggered_by}` : "operador local"}
              </p>
            </div>
            <span className="tnum hidden items-center gap-1 font-mono text-[10px] text-muted-foreground md:flex">
              {d.rev.slice(0, 8)}
              <CopyButton text={d.rev} />
            </span>
            <span className="tnum hidden text-right font-mono text-[10px] text-muted-foreground md:block">
              {dur(d.started_at, d.finished_at)}
            </span>
            <span className="tnum hidden text-right font-mono text-[10px] text-muted-foreground md:block">
              {d.status === "running" ? "en curso…" : rel(d.finished_at || d.started_at)}
            </span>
            <span className="flex items-center justify-end gap-1">
              <button
                className="inline-flex items-center gap-1 font-mono text-[10px] text-muted-foreground hover:text-foreground md:hidden"
                onClick={(e) => {
                  e.stopPropagation();
                  navigate(`/app/${d.app}`);
                }}
                aria-label={`abrir app ${d.app}`}
              >
                app <ArrowRight className="h-3 w-3" />
              </button>
              <ArrowRight className="hidden h-3 w-3 text-muted-foreground/35 transition-transform group-hover:translate-x-0.5 group-hover:text-foreground md:block" />
            </span>
          </div>
        ))}
        {vis.length === 0 && (
          <div className="px-4 py-8 text-center">
            <p className="font-mono text-[11px] text-muted-foreground">sin salidas para este filtro</p>
            <p className="mt-1 font-mono text-[9px] text-muted-foreground/70">
              solo producción · no hay entornos que filtrar
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
