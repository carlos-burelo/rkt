import { useEffect, useRef, useState } from "react";
import { ExternalLink, RotateCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dot } from "../../shared/components/ui";

// Vista previa de la app servida (Vercel-like): iframe del dominio real.
// Si el navegador no puede incrustarla (X-Frame-Options, DNS, TLS), se
// dice claro y se ofrece abrir en pestaña; nada de fingir un screenshot.
export function AppPreview({ domain, running }: { domain: string; running: boolean }) {
  const scheme = window.location.protocol === "https:" ? "https" : "http";
  const src = `${scheme}://${domain}`;
  const [nonce, setNonce] = useState(0);
  const [state, setState] = useState<"loading" | "ok" | "blocked">("loading");
  const timer = useRef<number | null>(null);

  useEffect(() => {
    if (!running) return;
    setState("loading");
    if (timer.current) window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      setState((s) => (s === "loading" ? "blocked" : s));
    }, 8000);
    return () => {
      if (timer.current) window.clearTimeout(timer.current);
    };
  }, [src, nonce, running]);

  return (
    <section className="border border-border bg-card/30" aria-label="vista previa de la app">
      <header className="flex flex-wrap items-center gap-2 border-b border-border bg-secondary/20 px-3 py-2">
        <span aria-hidden className="h-1.5 w-1.5 bg-signal" />
        <h2 className="text-[12px] font-medium">Vista previa</h2>
        <span className="flex min-w-0 items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
          <Dot status={running ? "running" : "stopped"} />
          <span className="truncate">{domain}</span>
        </span>
        <span className="flex-1" />
        <Button
          variant="ghost"
          size="xs"
          onClick={() => setNonce((n) => n + 1)}
          aria-label="recargar vista previa"
          title="recargar vista previa"
        >
          <RotateCw className="h-3 w-3" /> recargar
        </Button>
        <a
          href={src}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:text-foreground"
        >
          abrir <ExternalLink className="h-3 w-3" />
        </a>
      </header>

      {!running && (
        <div className="px-4 py-10 text-center">
          <p className="micro text-muted-foreground/60">app detenida</p>
          <p className="mt-2 text-[12px]">La vista previa aparece cuando la unidad está en marcha.</p>
        </div>
      )}

      {running && state !== "blocked" && (
        <div className="relative">
          <iframe
            key={nonce}
            src={src}
            title={`vista previa de ${domain}`}
            className="block h-[360px] w-full bg-background"
            onLoad={() => setState("ok")}
          />
          {state === "loading" && (
            <div className="pointer-events-none absolute inset-0 flex items-center justify-center bg-background/70">
              <p className="font-mono text-[10px] text-muted-foreground">cargando {domain}…</p>
            </div>
          )}
        </div>
      )}

      {running && state === "blocked" && (
        <div className="px-4 py-8">
          <p className="micro text-warn">no se pudo incrustar</p>
          <p className="mt-2 text-[12px]">
            El navegador bloqueó el iframe o el dominio no resolvió desde esta máquina.
          </p>
          <p className="mt-1 font-mono text-[10px] leading-relaxed text-muted-foreground">
            revisa DNS/hosts para {domain} y que la app no envíe X-Frame-Options. La app sigue accesible en pestaña.
          </p>
          <a href={src} target="_blank" rel="noreferrer" className="mt-3 inline-block">
            <Button size="sm" variant="outline">
              <ExternalLink className="h-3 w-3" /> abrir {domain}
            </Button>
          </a>
        </div>
      )}
    </section>
  );
}
