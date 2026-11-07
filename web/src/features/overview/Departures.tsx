import { Dot } from "../../shared/components/ui";
import { dur, rel } from "../../shared/lib/time";
import { eventKindLabel } from "../../shared/lib/labels";
import { useI18n } from "../../shared/i18n";
import type { Deployment, RktEvent } from "../../shared/lib/types";
import { useUI } from "../../app/ui";

// Últimos despliegues (click abre detalle). Solo lectura del bus.
export function Departures({ deps }: { deps: Deployment[] }) {
  const { t } = useI18n();
  const setSel = useUI((s) => s.setSel);
  const recent = deps.slice(0, 6);
  return (
    <div className="divide-y divide-border/70">
      {recent.map((d) => (
        <button
          key={d.id}
          onClick={() => setSel(d)}
          className="group flex w-full items-center gap-2.5 px-3 py-2 text-left hover:bg-accent/45"
        >
          <Dot status={d.status} />
          <span className="min-w-0 flex-1">
            <span className="flex items-baseline gap-1.5">
              <span className="truncate font-mono text-[12px]">{d.app}</span>
              <span className="tnum shrink-0 font-mono text-[10px] text-muted-foreground">#{d.id}</span>
            </span>
            <span className="mt-0.5 block truncate font-mono text-[10px] text-muted-foreground">
              {d.rev.slice(0, 8)} · {dur(d.started_at, d.finished_at)}
            </span>
          </span>
          <span className="tnum shrink-0 font-mono text-[10px] text-muted-foreground">
            {d.status === "running" ? t("deploy.running") : rel(d.finished_at || d.started_at)}
          </span>
        </button>
      ))}
      {recent.length === 0 && (
        <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">{t("overview.departures.empty")}</p>
      )}
    </div>
  );
}

// Actividad reciente del supervisor. Solo lectura del bus.
export function Activity({ evs }: { evs: RktEvent[] }) {
  const { t } = useI18n();
  return (
    <div className="divide-y divide-border/60">
      {evs.slice(0, 8).map((e) => (
        <div key={e.id} className="grid grid-cols-[4.5rem_5rem_minmax(0,1fr)] items-baseline gap-2 px-3 py-1.5">
          <span className="tnum font-mono text-[10px] text-muted-foreground/60">{rel(e.ts)}</span>
          <span className="truncate font-mono text-[10px] tracking-[0.06em] text-muted-foreground uppercase">
            {eventKindLabel(e.kind)}
          </span>
          <span className="min-w-0 truncate font-mono text-[11px] text-foreground/70">
            {e.app && <span className="text-foreground/90">{e.app}</span>} {e.msg}
          </span>
        </div>
      ))}
      {evs.length === 0 && (
        <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">{t("overview.activity.empty")}</p>
      )}
    </div>
  );
}
