import { useState } from "react";
import { useNavigate } from "react-router";
import { ArrowRight, Search, X } from "lucide-react";
import { dur, rel } from "../../shared/lib/time";
import { Dot, StateRail } from "../../shared/components/ui";
import { CopyButton } from "../../shared/components/feedback";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { deployStateLabel } from "../../shared/lib/labels";
import { useUI } from "../../app/ui";
import { useFleet } from "../overview/hooks";
import { PageHeader, Segmented } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";

export type DepStatus = "all" | "ok" | "running" | "failed";

// Deployments: tablero de salidas. Columnas fijas (estado · salida ·
// revisión · duración · hora), filas ordenadas por tiempo, la fila en
// curso late. Duraciones tabulares alineadas a la derecha.
export function Deployments({ bare = false, status, max }: { bare?: boolean; status?: DepStatus; max?: number }) {
  const { t, tn } = useI18n();
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
          title={t("deployments.title")}
          description={t("deployments.description")}
          meta={<span className="tnum font-mono text-[10px] text-muted-foreground">
            {vis.length === base.length
              ? tn("deployments.count", base.length)
              : t("deployments.countFiltered", { shown: vis.length, total: base.length })}
          </span>}
        />
      )}
      {!bare && (
        <div className="flex flex-wrap items-center gap-2 border border-border bg-card/35 p-2">
          <div className="relative min-w-52 flex-1">
            <Search className="absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder={t("deployments.filter.placeholder")}
              className="h-7 border-transparent bg-transparent pl-8 font-mono text-[12px] hover:border-input"
              aria-label={t("deployments.filter.aria")}
            />
          </div>
          <Segmented
            value={st}
            onChange={setSt}
            label={t("deployments.filter.statusLabel")}
            items={[
              { value: "all", label: t("deployments.filter.all"), count: counts.all },
              { value: "ok", label: t("deployments.state.ok"), count: counts.ok },
              { value: "running", label: t("deploy.running"), count: counts.running },
              { value: "failed", label: t("deployments.filter.failed"), count: counts.failed },
            ]}
          />
          {(q !== "" || st !== "all") && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setQ("");
                setSt("all");
              }}
            >
              <X className="h-3 w-3" /> {t("deployments.filter.clear")}
            </Button>
          )}
        </div>
      )}
      <div className="border border-border" role="table" aria-label={t("deployments.table.aria")}>
        <div
          role="row"
          className={cn(
            "hidden grid-cols-[1.5rem_minmax(0,1fr)_8rem_4.5rem_5.5rem_1.5rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 lg:grid",
            !bare && "sticky top-12 z-10 bg-card",
          )}
        >
          <span role="columnheader" className="micro">{t("deployments.table.state")}</span>
          <span role="columnheader" className="micro">{t("deployments.table.output")}</span>
          <span role="columnheader" className="micro">{t("deployments.table.rev")}</span>
          <span role="columnheader" className="micro text-right">{t("deployments.table.duration")}</span>
          <span role="columnheader" className="micro text-right">{t("deployments.table.time")}</span>
          <span role="columnheader" className="sr-only">{t("deployments.table.actions")}</span>
        </div>
        {vis.map((d) => (
          <div
            key={d.id}
            onClick={() => setSel(d)}
            onKeyDown={(e) => {
              if (e.target !== e.currentTarget) return;
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                setSel(d);
              }
            }}
            tabIndex={0}
            aria-label={t("deployments.row.openAria", { app: d.app, id: d.id })}
            className={cn(
              "group relative grid cursor-pointer grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-3 border-b border-border/70 bg-card/35 px-3 py-2.5 text-left last:border-0 hover:bg-accent/40 focus-visible:bg-accent/40 lg:grid-cols-[1.5rem_minmax(0,1fr)_8rem_4.5rem_5.5rem_1.5rem]",
              d.status === "running" && "animate-row-arrive bg-signal/[0.035] hover:bg-signal/[0.06]",
              d.status === "failed" && "bg-bad/[0.045] hover:bg-bad/[0.075] focus-visible:bg-bad/[0.075]",
            )}
            role="row"
          >
            <StateRail status={d.status} />
            <span role="cell" className="flex items-center">
              <Dot status={d.status} pulse={d.status === "running"} />
            </span>
            <div role="cell" className="min-w-0">
              <p className="truncate font-mono text-[13px]">
                {d.app} <span className="tnum text-muted-foreground">#{d.id}</span>
              </p>
              <p className="tnum mt-0.5 truncate font-mono text-[10px] text-muted-foreground lg:hidden">
                {deployStateLabel(d.status)} · {d.rev.slice(0, 8)} · {dur(d.started_at, d.finished_at)} ·{" "}
                {d.status === "running" ? t("deployments.runningEllipsis") : rel(d.finished_at || d.started_at)}
              </p>
              <p className="mt-0.5 hidden truncate font-mono text-[10px] text-muted-foreground lg:block">
                <span
                  className={
                    d.status === "failed"
                      ? "text-bad"
                      : d.status === "running"
                        ? "text-signal"
                        : d.status === "ok"
                          ? "text-ok/80"
                          : ""
                  }
                >
                  {deployStateLabel(d.status)}
                </span>
                {" · "}
                {d.triggered_by ? t("deployments.row.by", { user: d.triggered_by }) : t("deployments.row.localOperator")}
              </p>
            </div>
            <span role="cell" className="tnum hidden items-center gap-1 font-mono text-[11px] text-muted-foreground lg:flex">
              {d.rev.slice(0, 8)}
              <CopyButton text={d.rev} />
            </span>
            <span role="cell" className="tnum hidden text-right font-mono text-[11px] text-muted-foreground lg:block">
              {dur(d.started_at, d.finished_at)}
            </span>
            <span role="cell" className="tnum hidden text-right font-mono text-[11px] text-muted-foreground lg:block">
              {d.status === "running" ? t("deployments.runningEllipsis") : rel(d.finished_at || d.started_at)}
            </span>
            <span role="cell" className="flex items-center justify-end gap-1">
              <button
                className="-my-1 inline-flex items-center gap-1 px-1 py-1.5 font-mono text-[10px] text-muted-foreground hover:text-foreground lg:hidden"
                onClick={(e) => {
                  e.stopPropagation();
                  navigate(`/app/${d.app}`);
                }}
                aria-label={t("deployments.row.openAppAria", { app: d.app })}
              >
                {t("deployments.row.app")} <ArrowRight className="h-3 w-3" />
              </button>
              <ArrowRight aria-hidden className="hidden h-3 w-3 text-muted-foreground/35 transition-transform group-hover:translate-x-0.5 group-hover:text-foreground lg:block" />
            </span>
          </div>
        ))}
        {vis.length === 0 && (
          <div className="px-4 py-8 text-center">
            <p className="font-mono text-[11px] text-muted-foreground">{t("deployments.empty.title")}</p>
            <p className="mt-1 font-mono text-[9px] text-muted-foreground/70">
              {t("deployments.empty.hint")}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
