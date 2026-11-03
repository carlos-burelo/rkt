import { Root, Backdrop, Popup, Title, Close } from "@base_ui/react/dialog";
import { useMemo } from "react";
import { Deployment } from "../../shared/lib/types";
import { rel, dur } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { CopyButton } from "../../shared/components/feedback";
import { Button } from "@/components/ui/button";
import { Timeline } from "./Timeline";

// steps checklist CI desde marcadores "▶/✔/✘ step" del build log.
function Steps({ log }: { log: string }) {
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
    <div className="space-y-1">
      {steps.map((s, i) => (
        <p key={i} className="flex items-center gap-2 font-mono text-xs">
          <span
            className={`inline-block h-2 w-2 rounded-full ${
              s.state === "ok" ? "bg-emerald-400" : s.state === "bad" ? "bg-red-400" : "bg-amber-400 animate-pulse"
            }`}
          />
          <span className="truncate">{s.name}</span>
        </p>
      ))}
    </div>
  );
}

export function DeployDrawer({
  dep,
  buildLog,
  canWrite,
  onAction,
  onRetry,
  onClose,
}: {
  dep: Deployment | null;
  buildLog: string;
  canWrite: boolean;
  onAction: (app: string, a: "restart" | "deploy" | "rollback") => void;
  onRetry: (id: number) => void;
  onClose: () => void;
}) {
  return (
    <Root
      open={dep !== null}
      onOpenChange={(open: boolean) => {
        if (!open) onClose();
      }}
    >
      <Backdrop className="fixed inset-0 bg-black/70" />
      <Popup className="fixed right-0 top-0 flex h-full w-full max-w-xl flex-col border-l border-zinc-800 bg-zinc-950">
        {dep && (
          <div className="flex h-full flex-col space-y-4 overflow-hidden p-6">
            <div className="flex items-center gap-2">
              <Dot status={dep.status} />
              <Title className="font-mono text-base">
                {dep.app} <span className="text-zinc-500">#{dep.id}</span>
              </Title>
              <span className="flex-1" />
              <Close className="rounded border border-zinc-800 px-2 py-0.5 text-xs text-zinc-400 hover:text-zinc-100">
                cerrar
              </Close>
            </div>
            <dl className="grid grid-cols-2 gap-1.5 font-mono text-xs">
              <dt className="text-zinc-500">status</dt><dd>{dep.status}</dd>
              <dt className="text-zinc-500">rev</dt><dd className="truncate">{dep.rev} <CopyButton text={dep.rev} /></dd>
              <dt className="text-zinc-500">inicio</dt><dd>{rel(dep.started_at)}</dd>
              <dt className="text-zinc-500">duración</dt><dd>{dur(dep.started_at, dep.finished_at)}</dd>
              <dt className="text-zinc-500">actor</dt><dd>{dep.triggered_by || "—"}</dd>
            </dl>
            {dep.env_snapshot && dep.env_snapshot !== "{}" && (
              <details className="rounded-lg border border-zinc-800 p-3">
                <summary className="cursor-pointer text-xs text-zinc-400">env snapshot</summary>
                <pre className="mt-2 overflow-auto font-mono text-[11px] text-zinc-500">{dep.env_snapshot}</pre>
              </details>
            )}
            {canWrite && (
              <div className="flex flex-wrap gap-2">
                {(["restart", "deploy", "rollback"] as const).map((a) => (
                  <Button key={a} variant="outline" size="sm" onClick={() => onAction(dep.app, a)}>
                    {a} {dep.app}
                  </Button>
                ))}
                <Button variant="outline" size="sm" onClick={() => onRetry(dep.id)}>
                  retry #{dep.id}
                </Button>
              </div>
            )}
            <p className="text-xs uppercase tracking-wider text-zinc-500">timeline</p>
            <Timeline dep={dep} />
            <Steps log={buildLog} />
            <p className="text-xs uppercase tracking-wider text-zinc-500">build log</p>
            <pre className="min-h-0 flex-1 overflow-auto rounded-lg border border-zinc-800 bg-black p-3 font-mono text-xs leading-relaxed text-zinc-300">
              {buildLog}
            </pre>
          </div>
        )}
      </Popup>
    </Root>
  );
}
