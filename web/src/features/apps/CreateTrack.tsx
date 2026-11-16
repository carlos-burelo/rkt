import { ArrowRight, Expand } from "lucide-react";
import { useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import { Dot } from "../../shared/components/ui";
import { useI18n } from "../../shared/i18n";
import { Steps } from "../deployments/steps";

function Cell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0 bg-card/45 px-3 py-2">
      <p className="micro">{label}</p>
      <div className="tnum mt-1 truncate font-mono text-[11px]">{children}</div>
    </div>
  );
}

// Seguimiento en vivo a pantalla completa: datos del despliegue a la
// izquierda (qué lleva) y registro a la derecha.
export function CreateTrack({
  app,
  deploymentId,
  kind,
  status,
  log,
  domain,
  command,
  originLabel,
  source,
  auto,
  onClose,
  onOpen,
}: {
  app: string;
  deploymentId: number;
  kind: "build" | "deploy";
  status: string;
  log: string;
  domain: string;
  command: string;
  originLabel: string;
  source: string;
  auto?: boolean;
  onClose: () => void;
  onOpen: () => void;
}) {
  const { t } = useI18n();
  const navigate = useNavigate();
  return (
    <div className="space-y-4">
      <div>
        <div className="flex items-center gap-2">
          <Dot status={status} pulse={status === "running"} />
          <span className="micro">{t(kind === "build" ? "create.track.build" : "create.track.deploy")}</span>
          <span className="flex-1" />
          <span className="tnum font-mono text-[9px] text-muted-foreground">
            {t(status === "running" ? "create.track.running" : status === "ok" ? "create.track.ok" : "create.track.failed")}
          </span>
        </div>
        <p className="mt-1 font-mono text-[18px]">
          {app} <span className="tnum text-muted-foreground">#{deploymentId}</span>
        </p>
        <p className="font-mono text-[10px] leading-relaxed text-muted-foreground">
          {t(
            status === "running"
              ? "create.track.runningHint"
              : status === "ok"
                ? "create.track.okHint"
                : "create.track.failedHint",
          )}
        </p>
      </div>

      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-[340px_minmax(0,1fr)]">
        <aside className="min-w-0 space-y-4 lg:sticky lg:top-12">
          <section className="border border-border bg-card/30 p-4">
            <p className="micro mb-3">{t("create.track.details")}</p>
            <div className="grid grid-cols-2 gap-px border border-border bg-border">
              <Cell label={t("deployments.table.output")}>{deploymentId}</Cell>
              <Cell label={t("deployments.table.state")}>
                {t(status === "running" ? "create.track.running" : status === "ok" ? "create.track.ok" : "create.track.failed")}
              </Cell>
              <Cell label={t("create.domain")}>{domain || "—"}</Cell>
              <Cell label={t("create.command")}>{command || t("create.review.detectedCommand")}</Cell>
              <Cell label={t("create.review.origin")}>{originLabel}</Cell>
              <Cell label={t("create.repo")}>{source || "—"}</Cell>
              {auto !== undefined && (
                <Cell label={t("create.autoDeploy")}>{t(auto ? "create.review.autoYes" : "create.review.autoNo")}</Cell>
              )}
            </div>
          </section>
          <section className="border border-border bg-card/30 p-4">
            <p className="micro mb-3">{t("deployments.buildSteps")}</p>
            {log ? (
              <Steps log={log} />
            ) : (
              <p className="font-mono text-[10px] text-muted-foreground">{t("deployments.noSteps")}</p>
            )}
          </section>
        </aside>

        <section className="min-w-0 border border-border">
          <header className="flex items-center gap-2 border-b border-console-border bg-console-raised px-4 py-2">
            <span className="micro text-console-muted">{t("deployments.buildLog")}</span>
            <span className="flex-1" />
            <span className="tnum font-mono text-[9px] text-console-muted">
              {log ? t("deployments.lines", { n: log.split("\n").length }) : t("deployments.logEmpty")}
            </span>
          </header>
          <pre className="console max-h-[70vh] min-h-[50vh] overflow-auto p-4 font-mono text-[11px] leading-relaxed whitespace-pre-wrap">
            {log || t("create.track.waiting")}
          </pre>
        </section>
      </div>

      <div className="flex items-center gap-2 border-t border-border pt-3">
        <Button variant="ghost" size="sm" onClick={onClose}>
          {t(status === "running" ? "create.track.closeRunning" : "common.close")}
        </Button>
        <span className="flex-1" />
        <Button variant="outline" size="sm" onClick={() => navigate(`/deployments/${deploymentId}`)}>
          <Expand className="h-3 w-3" /> {t("create.track.fullPage")}
        </Button>
        {status === "ok" && (
          <Button size="sm" onClick={onOpen}>
            {t("create.track.openApp")} <ArrowRight className="h-3 w-3" />
          </Button>
        )}
      </div>
    </div>
  );
}
