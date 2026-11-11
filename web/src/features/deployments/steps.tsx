import { useMemo } from "react";
import { useI18n } from "../../shared/i18n";

// Checklist CI desde marcadores "▶/✔/✘ step" del build log. Un solo hogar:
// lo usan el drawer de deploys y el seguimiento de creación.
export function Steps({ log }: { log: string }) {
  const { t } = useI18n();
  const steps = useMemo(() => {
    const out: { name: string; state: "ok" | "bad" | "run" }[] = [];
    for (const line of log.split("\n")) {
      if (line.startsWith("▶ step: ")) out.push({ name: line.slice(8), state: "run" });
      else if (line.startsWith("✔ step ok: ")) {
        const name = line.slice(11);
        const s = [...out].reverse().find((x) => x.name === name);
        if (s) s.state = "ok";
      } else if (line.startsWith("✘ step failed: ")) {
        const name = line.slice(15);
        const s = [...out].reverse().find((x) => x.name === name);
        if (s) s.state = "bad";
      }
    }
    return out;
  }, [log]);
  if (steps.length === 0) return null;
  return (
    <ol className="divide-y divide-border/60 border border-border">
      {steps.map((s, i) => (
        <li key={i} className="flex items-center gap-2 bg-card/30 px-2.5 py-1.5">
          <span className="folio w-5 shrink-0">{String(i + 1).padStart(2, "0")}</span>
          <span
            className={`h-1.5 w-1.5 shrink-0 ${
              s.state === "ok" ? "bg-ok" : s.state === "bad" ? "bg-bad" : "bg-warn animate-signal-blink"
            }`}
            aria-label={t(
              s.state === "ok"
                ? "deployments.stepAria.ok"
                : s.state === "bad"
                  ? "deployments.stepAria.bad"
                  : "deployments.stepAria.run",
            )}
          />
          <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{s.name}</span>
          <span
            className={`font-mono text-[9px] tracking-[0.08em] uppercase ${
              s.state === "run" ? "text-warn" : s.state === "ok" ? "text-ok" : "text-bad"
            }`}
          >
            {s.state === "run"
              ? t("deployments.step.run")
              : s.state === "ok"
                ? t("deployments.step.ok")
                : t("deployments.step.failed")}
          </span>
        </li>
      ))}
    </ol>
  );
}
