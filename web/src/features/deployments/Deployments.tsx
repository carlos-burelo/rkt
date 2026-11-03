import { useState } from "react";
import { useNavigate } from "react-router";
import { ArrowRight, Search } from "lucide-react";
import { dur, rel } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { CopyButton } from "../../shared/components/feedback";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { useUI } from "../../app/ui";
import { useFleet } from "../overview/hooks";

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
  let vis = deps.filter(
    (d) =>
      (appFilter === "" || d.app === appFilter) &&
      (st === "all" || d.status === st) &&
      (f === "" || d.app.toLowerCase().includes(f) || d.rev.toLowerCase().includes(f)),
  );
  if (max !== undefined) vis = vis.slice(0, max);
  return (
    <div className="space-y-3">
      {!bare && (
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-52 flex-1">
            <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Filtrar por app o rev…"
              className="pl-8 font-mono text-xs"
              aria-label="filtrar deployments"
            />
          </div>
          <div className="flex gap-1.5" role="group" aria-label="filtrar por estado">
            {(["all", "ok", "running", "failed"] as const).map((s) => (
              <Badge
                key={s}
                variant={st === s ? "default" : "outline"}
                className="cursor-pointer font-mono text-[11px] font-normal"
                onClick={() => setSt(s)}
              >
                {s === "all" ? "todos" : s}
              </Badge>
            ))}
          </div>
        </div>
      )}
      <div className="overflow-hidden rounded-lg border" role="table" aria-label="tablero de deployments">
        <div className="tnum hidden grid-cols-[2rem_1fr_10rem_5rem_7rem] items-center gap-3 border-b bg-muted/40 px-4 py-1.5 font-mono text-[10px] uppercase tracking-wider text-muted-foreground sm:grid" aria-hidden>
          <span>est</span>
          <span>salida</span>
          <span>rev</span>
          <span className="text-right">dur</span>
          <span className="text-right">hora</span>
        </div>
        {vis.map((d) => (
          <div
            key={d.id}
            onClick={() => setSel(d)}
            className={`grid w-full cursor-pointer grid-cols-[2rem_1fr_auto] items-center gap-3 border-b bg-card/40 px-4 py-2.5 text-left last:border-0 hover:bg-accent/40 sm:grid-cols-[2rem_1fr_10rem_5rem_7rem] ${d.status === "running" ? "animate-row-arrive" : ""}`}
            role="row"
          >
            <Dot status={d.status} />
            <div className="min-w-0">
              <p className="truncate font-mono text-xs">
                {d.app} <span className="text-muted-foreground">· #{d.id}</span>
              </p>
              <p className="tnum truncate font-mono text-[11px] text-muted-foreground sm:hidden">
                {d.rev.slice(0, 8)} · {d.status === "running" ? "en curso…" : rel(d.finished_at || d.started_at)}
              </p>
              {d.triggered_by && (
                <p className="hidden truncate font-mono text-[11px] text-muted-foreground sm:block">por {d.triggered_by}</p>
              )}
            </div>
            <span className="tnum hidden items-center gap-1 font-mono text-[11px] text-muted-foreground sm:flex">
              {d.rev.slice(0, 8)}
              <CopyButton text={d.rev} />
            </span>
            <span className="tnum hidden text-right font-mono text-[11px] text-muted-foreground sm:block">
              {dur(d.started_at, d.finished_at)}
            </span>
            <span className="tnum hidden text-right font-mono text-[11px] text-muted-foreground sm:block">
              {d.status === "running" ? "en curso…" : rel(d.finished_at || d.started_at)}
            </span>
            <button
              className="inline-flex items-center gap-1 rounded px-2 py-1 font-mono text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground sm:hidden"
              onClick={(e) => {
                e.stopPropagation();
                navigate(`/app/${d.app}`);
              }}
              aria-label={`abrir app ${d.app}`}
            >
              app <ArrowRight className="h-3 w-3 inline" />
            </button>
          </div>
        ))}
        {vis.length === 0 && (
          <p className="p-6 font-mono text-xs text-muted-foreground">Sin salidas para este filtro. Production-only: no hay environments que filtrar.</p>
        )}
      </div>
    </div>
  );
}
