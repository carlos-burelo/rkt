import { ArrowRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useI18n } from "../../shared/i18n";
import { Steps } from "../deployments/steps";

// Panel de seguimiento en vivo de un build/deploy lanzado al crear la app.
export function CreateTrack({
  app,
  kind,
  status,
  log,
  onClose,
  onOpen,
}: {
  app: string;
  kind: "build" | "deploy";
  status: string;
  log: string;
  onClose: () => void;
  onOpen: () => void;
}) {
  const { t } = useI18n();
  return (
    <div className="space-y-3">
      <div>
        <div className="flex items-center gap-2 pr-8">
          <span
            aria-hidden
            className={`h-1.5 w-1.5 ${
              status === "running" ? "animate-pulse bg-signal" : status === "ok" ? "bg-ok" : "bg-bad"
            }`}
          />
          <span className="micro">{t(kind === "build" ? "create.track.build" : "create.track.deploy")}</span>
          <span className="flex-1" />
          <span className="tnum font-mono text-[9px] text-muted-foreground">
            {t(status === "running" ? "create.track.running" : status === "ok" ? "create.track.ok" : "create.track.failed")}
          </span>
        </div>
        <p className="mt-1 font-mono text-[18px]">{app}</p>
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
      <Steps log={log} />
      <pre className="max-h-64 overflow-auto border border-border bg-background p-3 font-mono text-[10px] leading-relaxed whitespace-pre-wrap">
        {log || t("create.track.waiting")}
      </pre>
      <div className="flex items-center gap-2 border-t border-border pt-3">
        <Button variant="ghost" size="sm" onClick={onClose}>
          {t(status === "running" ? "create.track.closeRunning" : "common.close")}
        </Button>
        <span className="flex-1" />
        {status === "ok" && (
          <Button size="sm" onClick={onOpen}>
            {t("create.track.openApp")} <ArrowRight className="h-3 w-3" />
          </Button>
        )}
      </div>
    </div>
  );
}
