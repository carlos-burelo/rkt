import { Deployment } from "../../shared/lib/types";
import { rel, dur } from "../../shared/lib/time";

// Timeline ciclo de vida del deploy (started → building → fin).
export function Timeline({ dep }: { dep: Deployment }) {
  const steps: { label: string; sub: string; state: "ok" | "run" | "bad" | "idle" }[] = [
    { label: "Inicio", sub: rel(dep.started_at), state: "ok" },
    dep.status === "running"
      ? { label: "Building", sub: `en curso · ${dur(dep.started_at, "")}`, state: "run" }
      : { label: "Build", sub: dur(dep.started_at, dep.finished_at), state: dep.status === "ok" ? "ok" : "bad" },
    dep.status === "running"
      ? { label: "Fin", sub: "pendiente", state: "idle" }
      : { label: dep.status === "ok" ? "Live" : "Fallido", sub: rel(dep.finished_at), state: dep.status === "ok" ? "ok" : "bad" },
  ];
  const color = (s: string) =>
    s === "ok" ? "bg-emerald-400" : s === "run" ? "bg-emerald-400 animate-pulse" : s === "bad" ? "bg-red-400" : "bg-muted-foreground/50";
  return (
    <ol className="space-y-0">
      {steps.map((s, i) => (
        <li key={s.label} className="flex gap-3">
          <div className="flex flex-col items-center">
            <span className={`mt-1 h-2.5 w-2.5 rounded-full ${color(s.state)}`} />
            {i < steps.length - 1 && <span className="w-px flex-1 bg-border" />}
          </div>
          <div className="pb-4">
            <p className="text-xs font-medium">{s.label}</p>
            <p className="tnum font-mono text-[11px] text-muted-foreground">{s.sub}</p>
          </div>
        </li>
      ))}
    </ol>
  );
}
