import { useEffect, useRef, useState } from "react";
import { ExternalLink, RotateCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dot } from "../../shared/components/ui";
import { useFleet } from "../overview/hooks";
import { publicUrl, publicHost } from "../../shared/lib/url";
import { useI18n } from "../../shared/i18n";

// Vista previa en miniatura 16:9: el sitio se renderiza en un viewport
// virtual 1280x720 y se escala completo al ancho del marco. No recorta.
const PREVIEW_WIDTHS = { s: 360, m: 560, l: 800 } as const;
type PreviewSize = keyof typeof PREVIEW_WIDTHS;

const FRAME_W = 1280;
const FRAME_H = 720;

function initialSize(): PreviewSize {
  const saved = localStorage.getItem("rkt.preview.size");
  return saved === "m" || saved === "l" ? saved : "s";
}

// MiniFrame escala el iframe 1280x720 al ancho disponible manteniendo 16:9.
function MiniFrame({
  src,
  nonce,
  domain,
  onLoad,
}: {
  src: string;
  nonce: number;
  domain: string;
  onLoad: () => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(0);
  const { t } = useI18n();

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const fit = () => setScale(el.clientWidth / FRAME_W);
    fit();
    const ro = new ResizeObserver(fit);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  return (
    <div ref={box} className="relative aspect-video w-full overflow-hidden bg-background">
      <iframe
        key={nonce}
        src={src}
        title={t("apps.preview.frameTitle", { domain })}
        loading="lazy"
        onLoad={onLoad}
        style={{
          width: FRAME_W,
          height: FRAME_H,
          transform: `scale(${scale})`,
          transformOrigin: "top left",
          pointerEvents: "none",
        }}
        className="absolute top-0 left-0 border-0"
      />
    </div>
  );
}

export function AppPreview({ domain, running }: { domain: string; running: boolean }) {
  const { t } = useI18n();
  const { status } = useFleet();
  const src = publicUrl(domain, status);
  const [nonce, setNonce] = useState(0);
  const [size, setSize] = useState<PreviewSize>(initialSize);
  const [state, setState] = useState<"loading" | "ok" | "blocked">("loading");
  const timer = useRef<number | null>(null);

  const changeSize = (s: PreviewSize) => {
    setSize(s);
    localStorage.setItem("rkt.preview.size", s);
  };

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
    <section className="border border-border bg-card/30" aria-label={t("apps.preview.aria")}>
      <header className="flex flex-wrap items-center gap-2 border-b border-border bg-secondary/20 px-3 py-2">
        <span aria-hidden className="h-1.5 w-1.5 bg-signal" />
        <h2 className="text-[12px] font-medium">{t("apps.preview.title")}</h2>
        <span className="flex min-w-0 items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
          <Dot status={running ? "running" : "stopped"} />
          <span className="truncate">{publicHost(domain, status)}</span>
        </span>
        <span className="flex-1" />
        <div className="flex items-center gap-0.5" role="group" aria-label={t("apps.preview.sizeAria")}>
          {(["s", "m", "l"] as const).map((s) => (
            <button
              key={s}
              onClick={() => changeSize(s)}
              title={t(`apps.preview.size.${s}`)}
              className={
                size === s
                  ? "border border-signal/60 bg-signal/10 px-1.5 font-mono text-[9px] uppercase text-signal"
                  : "border border-transparent px-1.5 font-mono text-[9px] uppercase text-muted-foreground/60 hover:text-foreground"
              }
            >
              {s}
            </button>
          ))}
        </div>
        <Button
          variant="ghost"
          size="xs"
          onClick={() => setNonce((n) => n + 1)}
          aria-label={t("apps.preview.reload")}
          title={t("apps.preview.reload")}
        >
          <RotateCw className="h-3 w-3" /> {t("apps.preview.reloadAction")}
        </Button>
        <a
          href={src}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:text-foreground"
        >
          {t("common.open")} <ExternalLink className="h-3 w-3" />
        </a>
      </header>

      {!running && (
        <div className="px-4 py-10 text-center">
          <p className="micro text-muted-foreground/60">{t("apps.preview.stopped")}</p>
          <p className="mt-2 text-[12px]">{t("apps.preview.stoppedHint")}</p>
        </div>
      )}

      {running && state !== "blocked" && (
        <div className="relative p-2" style={{ maxWidth: PREVIEW_WIDTHS[size] }}>
          <div className="relative">
            <MiniFrame src={src} nonce={nonce} domain={domain} onLoad={() => setState("ok")} />
            {state === "ok" && (
              <a
                href={src}
                target="_blank"
                rel="noreferrer"
                aria-label={t("apps.preview.openAria", { domain })}
                title={t("apps.preview.openTitle", { domain })}
                className="absolute inset-0 z-10 flex items-end justify-end bg-transparent p-2 opacity-0 transition-opacity hover:opacity-100 focus-visible:opacity-100"
              >
                <span className="border border-border bg-background/90 px-2 py-1 font-mono text-[9px] tracking-[0.08em] uppercase">
                  {t("common.open")} <ExternalLink className="inline h-3 w-3" />
                </span>
              </a>
            )}
            {state === "loading" && (
              <div className="pointer-events-none absolute inset-0 flex items-center justify-center bg-background/70">
                <p className="font-mono text-[10px] text-muted-foreground">{t("apps.preview.loading", { domain })}</p>
              </div>
            )}
          </div>
        </div>
      )}

      {running && state === "blocked" && (
        <div className="px-4 py-8">
          <p className="micro text-warn">{t("apps.preview.blocked")}</p>
          <p className="mt-2 text-[12px]">
            {t("apps.preview.blockedDesc")}
          </p>
          <p className="mt-1 font-mono text-[10px] leading-relaxed text-muted-foreground">
            {t("apps.preview.blockedHint", { domain })}
          </p>
          <a href={src} target="_blank" rel="noreferrer" className="mt-3 inline-block">
            <Button size="sm" variant="outline">
              <ExternalLink className="h-3 w-3" /> {t("apps.preview.openDomain", { domain })}
            </Button>
          </a>
        </div>
      )}
    </section>
  );
}
