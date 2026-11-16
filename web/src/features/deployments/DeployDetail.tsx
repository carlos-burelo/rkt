import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { errorToast } from "../../shared/components/feedback";
import { ArrowLeft } from "lucide-react";
import { rel, dur } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { deployStateLabel } from "../../shared/lib/labels";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { TechIcon } from "../../shared/components/TechIcon";
import { stackOfApp } from "../apps/model";
import { CopyButton } from "../../shared/components/feedback";
import { Button } from "@/components/ui/button";
import { PageHeader } from "../../shared/components/patterns";
import { streamLog, bust } from "../../shared/lib/http";
import { cancelDeployment, deploymentLog, diffDeployment, getDeployment, type DiffFile } from "./api";
import { getEnvKeys } from "../apps/api";
import { Timeline } from "./Timeline";
import { Steps } from "./steps";
import { useUI } from "../../app/ui";
import { useFleet } from "../overview/hooks";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import type { Deployment } from "../../shared/lib/types";

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

// Detalle de despliegue como página completa (estilo Vercel): cabecera con
// estado y acciones, columna principal con log amplio y lateral con ciclo
// de vida + cambios. Un solo hogar: lo usa DeployPage (ruta /deployments/:id).
export function DeployDetail({ id }: { id: number }) {
  const { t, tn } = useI18n();
  const navigate = useNavigate();
  const { deps, metas } = useFleet();
  const canWrite = useAuth((a) => a.canWrite);
  const act = useUI((u) => u.act);
  const retry = useUI((u) => u.retry);
  const [fetched, setFetched] = useState<Deployment | null>(null);
  const [fetchErr, setFetchErr] = useState("");
  const [log, setLog] = useState(t("common.loading"));
  const [diff, setDiff] = useState<DiffFile[] | null>(null);
  const [diffErr, setDiffErr] = useState("");
  const [envKeys, setEnvKeys] = useState<string[] | null>(null);
  const [envErr, setEnvErr] = useState(false);

  const live = deps.find((d) => d.id === id) ?? null;
  const dep = live ?? fetched;

  // Sin el deploy en el bus (deep link o historial largo): carga puntual.
  useEffect(() => {
    if (live || fetched || fetchErr) return;
    let alive = true;
    getDeployment(id)
      .then((d) => { if (alive) setFetched(d); })
      .catch((e) => { if (alive) setFetchErr(e instanceof Error ? e.message : t("common.error")); });
    return () => { alive = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, live !== null]);

  // Log: SSE en vivo si corre; puntual si terminó.
  useEffect(() => {
    if (!dep) return;
    if (dep.status === "running") {
      let acc = "";
      return streamLog(
        `/api/v1/deployments/${dep.id}/log?stream=1&n=400`,
        (text, append) => {
          acc = append ? acc + text : text;
          setLog(acc);
        },
        (status) => {
          bust("fleet:");
          if (status !== "done") return;
          getDeployment(dep.id).then((d) => {
            if (d.status === "ok") setFetched(d);
          }).catch(() => {});
        },
      );
    }
    let alive = true;
    setLog(t("common.loading"));
    deploymentLog(dep.id)
      .then((text) => { if (alive) setLog(text); })
      .catch((e) => { if (alive) setLog(e instanceof Error ? e.message : t("shared.log.none")); });
    return () => { alive = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
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

  // Env que recibe el deploy: solo nombres (los valores nunca salen del
  // servidor); vigentes de la app, no foto del deploy.
  useEffect(() => {
    setEnvKeys(null);
    setEnvErr(false);
    if (!dep) return;
    let alive = true;
    getEnvKeys(dep.app)
      .then((keys) => { if (alive) setEnvKeys(keys); })
      .catch(() => { if (alive) setEnvErr(true); });
    return () => { alive = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dep?.id, dep?.app]);

  if (fetchErr) {
    return (
      <div className="space-y-4">
        <PageHeader
          title={t("deployments.detail.notFound")}
          description={t("deployments.detail.notFoundHint", { id })}
          meta={
            <Button variant="ghost" size="sm" onClick={() => navigate("/deployments")}>
              <ArrowLeft className="h-3 w-3" /> {t("deployments.detail.back")}
            </Button>
          }
        />
        <p className="border border-bad/40 bg-card/60 px-4 py-3 font-mono text-[11px] text-bad">{fetchErr}</p>
      </div>
    );
  }

  if (!dep) {
    return (
      <div className="space-y-4">
        <PageHeader
          title={t("deployments.detail.loading", { id })}
          description={t("deployments.detail.loadingHint")}
        />
        <p className="font-mono text-[11px] text-muted-foreground">{t("common.loading")}</p>
      </div>
    );
  }

  const depStack = (() => {
    const m = metas.find((x) => x.name === dep.app);
    const k = m ? stackOfApp(m) : "unknown";
    return k !== "unknown" && k !== "external" ? k : null;
  })();

  // Puerto resultante del corte: solo se conoce para el despliegue vigente
  // (la flota viva); los antiguos publicaron en puertos ya liberados.
  const livePort = (() => {
    const latest = deps.find((d) => d.app === dep.app) ?? null;
    if (!latest || latest.id !== dep.id) return "";
    const m = metas.find((x) => x.name === dep.app);
    return m?.port ? `:${m.port}` : "";
  })();

  return (
    <div className="space-y-4">
      <header className="flex flex-col gap-3 border-b border-border pb-4 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2.5">
            <span aria-hidden className="h-1 w-4 shrink-0 bg-signal" />
            <Dot status={dep.status} pulse={dep.status === "running"} />
            {depStack && <TechIcon kind={depStack} size={20} />}
            <h1 className="truncate text-[24px] leading-none font-semibold tracking-[-0.03em]">
              {dep.app} <span className="tnum text-muted-foreground">#{dep.id}</span>
            </h1>
          </div>
          <p className="mt-2 flex flex-wrap items-center gap-1.5 font-mono text-[11px] text-muted-foreground">
            <span
              className={
                dep.status === "failed"
                  ? "text-bad"
                  : dep.status === "running"
                    ? "text-signal"
                    : dep.status === "ok"
                      ? "text-ok/80"
                      : ""
              }
            >
              {deployStateLabel(dep.status)}
            </span>
            <span aria-hidden>·</span>
            <span className="tnum">{dep.rev.slice(0, 12)}</span>
            <CopyButton text={dep.rev} />
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2 sm:justify-end">
          <Button variant="ghost" size="sm" onClick={() => navigate("/deployments")}>
            <ArrowLeft className="h-3 w-3" /> {t("deployments.detail.back")}
          </Button>
        </div>
      </header>

      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-[340px_minmax(0,1fr)]">
        <aside className="min-w-0 space-y-4 lg:sticky lg:top-12">
          <div className="grid grid-cols-2 gap-px border border-border bg-border">
            <Cell label={t("deployments.started")}>{rel(dep.started_at)}</Cell>
            <Cell label={t("deployments.duration")}>{dur(dep.started_at, dep.finished_at)}</Cell>
            <Cell label={t("deployments.actor")}>{dep.triggered_by || "—"}</Cell>
            <Cell label={t("deployments.finished")}>
              {dep.finished_at ? rel(dep.finished_at) : t("deployments.runningEllipsis")}
            </Cell>
            <Cell label={t("deployments.detail.port")}>{livePort || "—"}</Cell>
          </div>
          <p className="px-3 py-2 font-mono text-[9px] leading-relaxed text-muted-foreground/70">
            {t("deployments.detail.portHint")}
          </p>

          <div className="flex flex-wrap gap-2 border border-border bg-card/30 p-3">
            <Button variant="outline" size="sm" onClick={() => navigate(`/app/${dep.app}`)}>
              {t("deployments.openApp")}
            </Button>
            {canWrite && (
              <>
                {([
                  ["restart", "deployments.action.restart"],
                  ["deploy", "deployments.action.redeploy"],
                ] as const).map(([a, labelKey]) => (
                  <Button key={a} variant="outline" size="sm" onClick={() => act(dep.app, a)}>
                    {t(labelKey)}
                  </Button>
                ))}
                <ConfirmButton
                  label={t("deployments.action.rollback")}
                  confirmLabel={t("deployments.action.rollbackConfirm")}
                  onConfirm={() => act(dep.app, "rollback")}
                  size="sm"
                />
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
                        .catch((e) => errorToast(e instanceof Error ? e.message : t("common.error")));
                    }}
                  >
                    {t("deployments.cancel")}
                  </Button>
                )}
              </>
            )}
          </div>

          <section className="border border-border bg-card/30 p-4">
            <p className="micro mb-3">{t("deployments.lifecycle")}</p>
            <Timeline dep={dep} />
          </section>
          <section className="border border-border bg-card/30 p-4">
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
                <div className="px-3 py-2">
                  <p className="font-mono text-[10px] text-warn">{diffErr}</p>
                  <p className="mt-1 font-mono text-[9px] text-muted-foreground">{t("deployments.diffFailedHint")}</p>
                </div>
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
          <section className="border border-border bg-card/30 p-4">
            <div className="flex items-center gap-2">
              <p className="micro">{t("deployments.env")}</p>
              <span className="flex-1" />
              <span className="tnum font-mono text-[9px] text-muted-foreground">
                {envKeys ? tn("deployments.changesCount", envKeys.length) : ""}
              </span>
            </div>
            <p className="mt-1.5 font-mono text-[9px] leading-relaxed text-muted-foreground/70">
              {t("deployments.envNote")}
            </p>
            <div className="mt-2 border border-border">
              {envErr ? (
                <p className="px-3 py-2 font-mono text-[10px] text-muted-foreground">
                  {t("deployments.envUnavailable")}
                </p>
              ) : envKeys === null ? (
                <p className="px-3 py-2 font-mono text-[10px] text-muted-foreground">{t("common.loading")}</p>
              ) : envKeys.length === 0 ? (
                <p className="px-3 py-2 font-mono text-[10px] text-muted-foreground">{t("deployments.noEnv")}</p>
              ) : (
                envKeys.map((k) => (
                  <div key={k} className="border-b border-border/60 px-3 py-1.5 last:border-0">
                    <span className="block truncate font-mono text-[10px]">{k}</span>
                  </div>
                ))
              )}
            </div>
          </section>
        </aside>

        <section className="min-w-0 space-y-4">
          <div className="border border-border">
            <header className="flex items-center gap-2 border-b border-console-border bg-console-raised px-4 py-2">
              <span className="micro text-console-muted">{t("deployments.buildLog")}</span>
              <span className="flex-1" />
              <span className="tnum font-mono text-[9px] text-console-muted">
                {log ? tn("deployments.lines", log.split("\n").length) : t("deployments.logEmpty")}
              </span>
            </header>
            <pre className="console max-h-[72vh] min-h-[50vh] overflow-auto p-4 font-mono text-[11px] leading-relaxed whitespace-pre-wrap">
              {log || t("deployments.noLog")}
            </pre>
          </div>
          <section className="border border-border bg-card/30 p-4">
            <p className="micro mb-3">{t("deployments.buildSteps")}</p>
            {log ? (
              <Steps log={log} />
            ) : (
              <p className="font-mono text-[10px] text-muted-foreground">{t("deployments.noSteps")}</p>
            )}
          </section>
        </section>
      </div>
    </div>
  );
}
