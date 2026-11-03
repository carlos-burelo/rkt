import { Root, Backdrop, Popup, Title, Close } from "@base_ui/react/dialog";
import { useMemo } from "react";
import { useNavigate } from "react-router";
import { rel, dur } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { CopyButton } from "../../shared/components/feedback";
import { Button } from "@/components/ui/button";
import { Timeline } from "./Timeline";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

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

export function DeployDrawer() {
  const sel = useUI((u) => u.sel);
  const buildLog = useUI((u) => u.buildLog);
  const act = useUI((u) => u.act);
  const retry = useUI((u) => u.retry);
  const setSel = useUI((u) => u.setSel);
  const canWrite = useAuth((a) => a.canWrite);
  const navigate = useNavigate();
  const dep = sel;
  return (
    <Root
      open={dep !== null}
      onOpenChange={(open: boolean) => {
        if (!open) setSel(null);
      }}
    >
      <Backdrop className="fixed inset-0 bg-black/70" />
      <Popup className="fixed right-0 top-0 flex h-full w-full max-w-xl flex-col border-l bg-card">
        {dep && (
          <div className="flex h-full flex-col space-y-4 overflow-hidden p-6">
            <div className="flex items-center gap-2">
              <Dot status={dep.status} />
              <Title className="tnum font-mono text-base">
                {dep.app} <span className="text-muted-foreground">#{dep.id}</span>
              </Title>
              <span className="flex-1" />
              <Close className="rounded border px-2 py-0.5 text-xs text-muted-foreground hover:text-foreground">
                cerrar
              </Close>
            </div>
            <dl className="tnum grid grid-cols-2 gap-1.5 font-mono text-xs">
              <dt className="text-muted-foreground">status</dt><dd>{dep.status}</dd>
              <dt className="text-muted-foreground">rev</dt><dd className="truncate">{dep.rev} <CopyButton text={dep.rev} /></dd>
              <dt className="text-muted-foreground">inicio</dt><dd>{rel(dep.started_at)}</dd>
              <dt className="text-muted-foreground">duración</dt><dd>{dur(dep.started_at, dep.finished_at)}</dd>
              <dt className="text-muted-foreground">actor</dt><dd>{dep.triggered_by || "—"}</dd>
            </dl>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => { setSel(null); navigate(`/app/${dep.app}`); }}>
                abrir app
              </Button>
              {canWrite && (
                <>
                  {(["restart", "deploy", "rollback"] as const).map((a) => (
                    <Button key={a} variant="outline" size="sm" onClick={() => act(dep.app, a)}>
                      {a}
                    </Button>
                  ))}
                  <Button variant="outline" size="sm" onClick={() => retry(dep.id)}>
                    retry #{dep.id}
                  </Button>
                </>
              )}
            </div>
            <p className="text-sm font-medium">Timeline</p>
            <Timeline dep={dep} />
            <Steps log={buildLog} />
            <p className="text-sm font-medium">Build log</p>
            <pre className="min-h-0 flex-1 overflow-auto rounded-lg border bg-card/40 p-3 font-mono text-xs leading-relaxed text-foreground">
              {buildLog}
            </pre>
          </div>
        )}
      </Popup>
    </Root>
  );
}
