import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { useMemo } from "react";
import { useNavigate } from "react-router";
import { rel, dur } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { CopyButton } from "../../shared/components/feedback";
import { Button } from "@/components/ui/button";
import { Timeline } from "./Timeline";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

const { Root, Portal, Backdrop, Popup, Title, Close } = DialogPrimitive;

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
    <ol className="divide-y divide-border/60 border border-border">
      {steps.map((s, i) => (
        <li key={i} className="flex items-center gap-2 bg-card/30 px-2.5 py-1.5">
          <span className="folio w-5 shrink-0">{String(i + 1).padStart(2, "0")}</span>
          <span
            className={`h-1.5 w-1.5 shrink-0 ${
              s.state === "ok" ? "bg-ok" : s.state === "bad" ? "bg-bad" : "bg-warn animate-signal-blink"
            }`}
            aria-label={`paso ${s.state}`}
          />
          <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{s.name}</span>
          <span
            className={`font-mono text-[9px] tracking-[0.08em] uppercase ${
              s.state === "ok" ? "text-ok" : s.state === "bad" ? "text-bad" : "text-warn"
            }`}
          >
            {s.state === "run" ? "en curso" : s.state === "ok" ? "ok" : "falló"}
          </span>
        </li>
      ))}
    </ol>
  );
}

function Cell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0 bg-card/45 px-3 py-2">
      <p className="micro">{label}</p>
      <div className="tnum mt-1 truncate font-mono text-[11px]">{children}</div>
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
      <Portal>
        <Backdrop className="fixed inset-0 z-40 bg-ink/75 backdrop-blur-sm" />
        <Popup className="fixed top-0 right-0 z-50 flex h-full w-full max-w-2xl flex-col border-l border-border bg-popover">
          {dep && (
          <>
            <header className="flex h-11 shrink-0 items-center gap-2 border-b border-border bg-secondary/20 px-4">
              <span aria-hidden className="h-1 w-4 bg-signal" />
              <span className="micro">manifiesto de salida</span>
              <span className="flex-1" />
              <Close className="border border-border px-2 py-0.5 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:border-input hover:text-foreground">
                cerrar
              </Close>
            </header>

            <div className="min-h-0 flex-1 overflow-auto">
              <div className="border-b border-border px-4 py-4">
                <div className="flex items-start gap-3">
                  <Dot status={dep.status} className="mt-1.5" />
                  <div className="min-w-0 flex-1">
                    <Title className="truncate font-mono text-[18px] leading-none font-medium tracking-tight">
                      {dep.app} <span className="tnum text-muted-foreground">#{dep.id}</span>
                    </Title>
                    <p className="mt-1.5 flex flex-wrap items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
                      <span>{dep.status}</span>
                      <span aria-hidden>·</span>
                      <span className="tnum">{dep.rev.slice(0, 12)}</span>
                      <CopyButton text={dep.rev} />
                    </p>
                  </div>
                </div>
                <div className="mt-4 grid grid-cols-2 gap-px border border-border bg-border sm:grid-cols-4">
                  <Cell label="inicio">{rel(dep.started_at)}</Cell>
                  <Cell label="duración">{dur(dep.started_at, dep.finished_at)}</Cell>
                  <Cell label="actor">{dep.triggered_by || "—"}</Cell>
                  <Cell label="fin">{dep.finished_at ? rel(dep.finished_at) : "en curso"}</Cell>
                </div>
                <div className="mt-3 flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      setSel(null);
                      navigate(`/app/${dep.app}`);
                    }}
                  >
                    abrir app
                  </Button>
                  {canWrite && (
                    <>
                      {(["restart", "deploy", "rollback"] as const).map((a) => (
                        <Button key={a} variant="outline" size="sm" onClick={() => act(dep.app, a)}>
                          {a}
                        </Button>
                      ))}
                      <Button variant="ghost" size="sm" onClick={() => retry(dep.id)}>
                        retry #{dep.id}
                      </Button>
                    </>
                  )}
                </div>
              </div>

              <div className="grid gap-px border-b border-border bg-border sm:grid-cols-2">
                <section className="bg-popover p-4">
                  <p className="micro mb-3">ciclo de vida</p>
                  <Timeline dep={dep} />
                </section>
                <section className="bg-popover p-4">
                  <p className="micro mb-3">pasos del build</p>
                  {buildLog ? (
                    <Steps log={buildLog} />
                  ) : (
                    <p className="font-mono text-[10px] text-muted-foreground">sin pasos registrados</p>
                  )}
                </section>
              </div>

              <section>
                <header className="flex items-center gap-2 border-b border-border bg-secondary/20 px-4 py-2">
                  <span className="micro">build log</span>
                  <span className="flex-1" />
                  <span className="tnum font-mono text-[9px] text-muted-foreground">
                    {buildLog ? `${buildLog.split("\n").length} líneas` : "vacío"}
                  </span>
                </header>
                <pre className="min-h-72 overflow-auto bg-ink/45 p-4 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-foreground/85">
                  {buildLog || "sin build log para esta salida"}
                </pre>
              </section>
            </div>
          </>
        )}
        </Popup>
      </Portal>
    </Root>
  );
}
