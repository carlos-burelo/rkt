import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { rel, dur } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { deployStateLabel } from "../../shared/lib/labels";
import { CopyButton } from "../../shared/components/feedback";
import { Button } from "@/components/ui/button";
import { streamLog, bust } from "../../shared/lib/http";
import { diffDeployment, getDeployment, cancelDeployment, type DiffFile } from "./api";
import { Timeline } from "./Timeline";
import { useUI } from "../../app/ui";
import { useFleet } from "../overview/hooks";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";

const { Root, Portal, Backdrop, Popup, Title, Close } = DialogPrimitive;

// steps checklist CI desde marcadores "▶/✔/✘ step" del build log.
function Steps({ log }: { log: string }) {
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
              s.state === "ok" ? "text-ok" : s.state === "bad" ? "text-bad" : "text-warn"
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

const CHANGE_LABEL: Record<string, string> = {
  A: "deployments.change.added",
  M: "deployments.change.modified",
  D: "deployments.change.deleted",
  R: "deployments.change.renamed",
  C: "deployments.change.copied",
  T: "deployments.change.type",
};

function Cell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0 bg-card/45 px-3 py-2">
      <p className="micro">{label}</p>
      <div className="tnum mt-1 truncate font-mono text-[11px]">{children}</div>
    </div>
  );
}

export function DeployDrawer() {
  const { t, tn } = useI18n();
  const sel = useUI((u) => u.sel);
  const buildLog = useUI((u) => u.buildLog);
  const act = useUI((u) => u.act);
  const retry = useUI((u) => u.retry);
  const setSel = useUI((u) => u.setSel);
  const canWrite = useAuth((a) => a.canWrite);
  const navigate = useNavigate();
  const { deps } = useFleet();
  const dep = sel ? (deps.find((d) => d.id === sel.id) ?? sel) : null;
  const [diff, setDiff] = useState<DiffFile[] | null>(null);
  const [diffErr, setDiffErr] = useState("");

  // Deploy en curso: SSE del build log. Al terminar, refresca las apps.
  useEffect(() => {
    if (!dep || dep.status !== "running") return;
    let acc = "";
    return streamLog(
      `/api/v1/deployments/${dep.id}/log?stream=1&n=400`,
      (text, append) => {
        acc = append ? acc + text : text;
        useUI.setState({ buildLog: acc });
      },
      async (status) => {
        bust("fleet:");
        if (status !== "done") return;
        const d = await getDeployment(dep.id).catch(() => null);
        if (d?.status !== "ok") return; // fallido: dejar el drawer abierto
        const app = dep.app;
        useUI.getState().setSel(null);
        navigate(`/app/${app}`);
      },
    );
  }, [dep?.id, dep?.status]);

  useEffect(() => {
    setDiff(null);
    setDiffErr("");
    if (!dep || !dep.prev_rev || !dep.rev || dep.status === "running") return;
    let alive = true;
    diffDeployment(dep.app, dep.prev_rev, dep.rev)
      .then((files) => { if (alive) setDiff(files); })
      .catch((e) => { if (alive) setDiffErr(e instanceof Error ? e.message : t("common.error")); });
    return () => { alive = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dep?.id, dep?.prev_rev, dep?.rev, dep?.status]);

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
              <span className="micro">{t("deployments.manifest")}</span>
              <span className="flex-1" />
              <Close className="border border-border px-2 py-0.5 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:border-input hover:text-foreground">
                {t("common.close")}
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
                      <span>{deployStateLabel(dep.status)}</span>
                      <span aria-hidden>·</span>
                      <span className="tnum">{dep.rev.slice(0, 12)}</span>
                      <CopyButton text={dep.rev} />
                    </p>
                  </div>
                </div>
                <div className="mt-4 grid grid-cols-2 gap-px border border-border bg-border sm:grid-cols-4">
                  <Cell label={t("deployments.started")}>{rel(dep.started_at)}</Cell>
                  <Cell label={t("deployments.duration")}>{dur(dep.started_at, dep.finished_at)}</Cell>
                  <Cell label={t("deployments.actor")}>{dep.triggered_by || "—"}</Cell>
                  <Cell label={t("deployments.finished")}>
                    {dep.finished_at ? rel(dep.finished_at) : t("deploy.running")}
                  </Cell>
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
                    {t("deployments.openApp")}
                  </Button>
                  {canWrite && (
                    <>
                      {([
                        ["restart", "deployments.action.restart", "outline"],
                        ["deploy", "deployments.action.redeploy", "outline"],
                        ["rollback", "deployments.action.rollback", "destructive"],
                      ] as const).map(([a, labelKey, variant]) => (
                        <Button key={a} variant={variant} size="sm" onClick={() => act(dep.app, a)}>
                          {t(labelKey)}
                        </Button>
                      ))}
                      <Button variant="ghost" size="sm" onClick={() => retry(dep.id)}>
                        {t("deployments.retryId", { id: dep.id })}
                      </Button>
                      {dep.status === "running" && (
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => {
                            cancelDeployment(dep.id)
                              .then(() => toast.success(t("deployments.cancelled", { id: dep.id })))
                              .catch((e) => toast.error(e instanceof Error ? e.message : t("common.error")));
                          }}
                        >
                          {t("deployments.cancel")}
                        </Button>
                      )}
                    </>
                  )}
                </div>
              </div>

              <div className="grid gap-px border-b border-border bg-border sm:grid-cols-2">
                <section className="bg-popover p-4">
                  <p className="micro mb-3">{t("deployments.lifecycle")}</p>
                  <Timeline dep={dep} />
                </section>
                <section className="bg-popover p-4">
                  <p className="micro mb-3">{t("deployments.buildSteps")}</p>
                  {buildLog ? (
                    <Steps log={buildLog} />
                  ) : (
                    <p className="font-mono text-[10px] text-muted-foreground">{t("deployments.noSteps")}</p>
                  )}
                </section>
              </div>

              <section className="border-b border-border px-4 py-4">
                <div className="flex items-center gap-2">
                  <p className="micro">{t("deployments.changes")}</p>
                  <span className="flex-1" />
                  <span className="tnum font-mono text-[9px] text-muted-foreground">
                    {diff ? tn("deployments.changesCount", diff.length) : ""}
                  </span>
                </div>
                <div className="mt-2 border border-border">
                  {!dep.prev_rev ? (
                    <p className="px-3 py-2 font-mono text-[10px] text-muted-foreground">
                      {t("deployments.noPrevRev")}
                    </p>
                  ) : diffErr ? (
                    <p className="px-3 py-2 font-mono text-[10px] text-warn">{diffErr}</p>
                  ) : diff === null ? (
                    <p className="px-3 py-2 font-mono text-[10px] text-muted-foreground">{t("common.loading")}</p>
                  ) : diff.length === 0 ? (
                    <p className="px-3 py-2 font-mono text-[10px] text-muted-foreground">{t("deployments.noChanges")}</p>
                  ) : (
                    diff.map((f, i) => {
                      const kind = CHANGE_LABEL[f.status[0] ?? ""] ?? "deployments.change.modified";
                      return (
                        <div
                          key={`${f.path}-${i}`}
                          className="grid grid-cols-[5.5rem_minmax(0,1fr)] items-baseline gap-2 border-b border-border/60 px-3 py-1.5 last:border-0"
                        >
                          <span className="font-mono text-[9px] tracking-[0.06em] text-muted-foreground uppercase">
                            {t(kind)}
                          </span>
                          <span className="min-w-0 truncate font-mono text-[10px]">
                            {f.from ? `${f.from} → ${f.path}` : f.path}
                          </span>
                        </div>
                      );
                    })
                  )}
                </div>
              </section>

              <section>
                <header className="flex items-center gap-2 border-b border-console-border bg-console-raised px-4 py-2">
                  <span className="micro text-console-muted">{t("deployments.buildLog")}</span>
                  <span className="flex-1" />
                  <span className="tnum font-mono text-[9px] text-console-muted">
                    {buildLog ? tn("deployments.lines", buildLog.split("\n").length) : t("deployments.logEmpty")}
                  </span>
                </header>
                <pre className="console min-h-72 overflow-auto p-4 font-mono text-[11px] leading-relaxed whitespace-pre-wrap">
                  {buildLog || t("deployments.noLog")}
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
