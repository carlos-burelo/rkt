import { Deployment } from "../../shared/lib/types";
import { rel, dur } from "../../shared/lib/time";
import { useI18n } from "../../shared/i18n";

// Timeline ciclo de vida del deploy (started → building → fin).
export function Timeline({ dep }: { dep: Deployment }) {
  const { t } = useI18n();
  const steps: { label: string; sub: string; state: "ok" | "run" | "bad" | "idle" }[] = [
    { label: t("deployments.timeline.start"), sub: rel(dep.started_at), state: "ok" },
    dep.status === "running"
      ? {
          label: t("deployments.timeline.building"),
          sub: t("deployments.timeline.runningSub", { dur: dur(dep.started_at, "") }),
          state: "run",
        }
      : {
          label: t("deployments.timeline.build"),
          sub: dur(dep.started_at, dep.finished_at),
          state: dep.status === "ok" ? "ok" : "bad",
        },
    dep.status === "running"
      ? { label: t("deployments.timeline.end"), sub: t("deployments.timeline.pending"), state: "idle" }
      : {
          label: dep.status === "ok" ? t("deployments.timeline.published") : t("deployments.timeline.failed"),
          sub: rel(dep.finished_at),
          state: dep.status === "ok" ? "ok" : "bad",
        },
  ];
  const mark = (s: string) =>
    s === "ok"
      ? "border-ok/50 bg-ok"
      : s === "run"
        ? "border-signal/50 bg-signal animate-signal-blink"
        : s === "bad"
          ? "border-bad/50 bg-bad"
          : "border-border bg-transparent";
  const text = (s: string) =>
    s === "ok" ? "text-ok" : s === "run" ? "text-signal" : s === "bad" ? "text-bad" : "text-muted-foreground";
  return (
    <ol className="space-y-0">
      {steps.map((s, i) => (
        <li key={s.label} className="flex gap-3">
          <div className="flex flex-col items-center">
            <span className={`mt-1 h-2 w-2 border ${mark(s.state)}`} />
            {i < steps.length - 1 && <span className="w-px flex-1 bg-border" />}
          </div>
          <div className="pb-3">
            <p className={`font-mono text-[10px] tracking-[0.08em] uppercase ${text(s.state)}`}>{s.label}</p>
            <p className="tnum mt-0.5 font-mono text-[10px] text-muted-foreground">{s.sub}</p>
          </div>
        </li>
      ))}
    </ol>
  );
}
