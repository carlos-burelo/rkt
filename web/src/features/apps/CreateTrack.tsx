import { ArrowRight } from "lucide-react";
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";

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
  return (
    <div className="space-y-3">
      <DialogHeader>
        <div className="flex items-center gap-2 pr-8">
          <span
            aria-hidden
            className={`h-1.5 w-1.5 ${
              status === "running" ? "animate-pulse bg-signal" : status === "ok" ? "bg-ok" : "bg-bad"
            }`}
          />
          <span className="micro">{kind === "build" ? "compilando" : "desplegando"}</span>
          <span className="flex-1" />
          <span className="tnum font-mono text-[9px] text-muted-foreground">
            {status === "running" ? "en curso" : status === "ok" ? "ok" : "fallido"}
          </span>
        </div>
        <DialogTitle className="mt-1 font-mono text-[18px]">{app}</DialogTitle>
        <DialogDescription className="font-mono text-[10px] leading-relaxed">
          {status === "running"
            ? "log en vivo · puedes cerrar y seguir en Despliegues"
            : status === "ok"
              ? "listo · la app quedó publicada y respondiendo"
              : "falló · revisa el log; no se publicó"}
        </DialogDescription>
      </DialogHeader>
      <pre className="max-h-64 overflow-auto border border-border bg-background p-3 font-mono text-[10px] leading-relaxed whitespace-pre-wrap">
        {log || "esperando salida…"}
      </pre>
      <div className="flex items-center gap-2 border-t border-border pt-3">
        <Button variant="ghost" size="sm" onClick={onClose}>
          {status === "running" ? "cerrar (sigue)" : "cerrar"}
        </Button>
        <span className="flex-1" />
        {status === "ok" && (
          <Button size="sm" onClick={onOpen}>
            abrir app <ArrowRight className="h-3 w-3" />
          </Button>
        )}
      </div>
    </div>
  );
}
