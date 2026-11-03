import { useEffect, useMemo, useRef, useState } from "react";
import { Search } from "lucide-react";
import { Deployment, AppMeta } from "../../shared/lib/types";
import { Dot, Kbd } from "../../shared/components/ui";
import { stateLabel } from "../lib/labels";

export interface PaletteTarget {
  kind: "page" | "app" | "deployment" | "action";
  label: string;
  hint: string;
  run: () => void;
}

const KIND_LABEL: Record<PaletteTarget["kind"], string> = {
  page: "página",
  app: "unidad",
  deployment: "salida",
  action: "acción",
};

// CommandPalette ⌘K: páginas, apps, deployments, acciones. Sin deps.
export function CommandPalette({
  open,
  setOpen,
  apps,
  metas,
  deps,
  goPage,
  openApp,
  openDeploy,
  onAction,
}: {
  open: boolean;
  setOpen: (b: boolean) => void;
  apps: { name: string; running: boolean; failed?: boolean }[];
  metas: AppMeta[];
  deps: Deployment[];
  goPage: (p: "overview" | "proxy" | "deployments" | "logs" | "routes" | "settings" | "users" | "analytics" | "observability" | "environment" | "ci" | "alerts") => void;
  openApp: (name: string) => void;
  openDeploy: (d: Deployment) => void;
  onAction: (app: string, a: "restart" | "deploy" | "rollback") => void;
}) {
  const [q, setQ] = useState("");
  const [idx, setIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setQ("");
      setIdx(0);
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [open]);

  const targets: PaletteTarget[] = useMemo(() => {
    const out: PaletteTarget[] = [];
    const pages: [string, "overview" | "proxy" | "deployments" | "logs" | "routes" | "settings" | "users" | "analytics" | "observability" | "environment" | "ci" | "alerts"][] = [
      ["Resumen", "overview"],
      ["Proxy", "proxy"],
      ["Despliegues", "deployments"],
      ["CI", "ci"],
      ["Registros", "logs"],
      ["Analítica", "analytics"],
      ["Observabilidad", "observability"],
      ["Alertas", "alerts"],
      ["Dominios", "routes"],
      ["Entorno", "environment"],
      ["Ajustes", "settings"],
      ["Usuarios", "users"],
    ];
    for (const [label, p] of pages) {
      out.push({ kind: "page", label: `Ir a ${label}`, hint: "página", run: () => goPage(p) });
    }
    for (const a of apps) {
      out.push({ kind: "app", label: a.name, hint: a.failed ? "failed" : a.running ? "running" : "stopped", run: () => openApp(a.name) });
    }
    for (const d of deps.slice(0, 20)) {
      out.push({
        kind: "deployment",
        label: `#${d.id} ${d.app} ${d.rev.slice(0, 8)}`,
        hint: d.status,
        run: () => openDeploy(d),
      });
    }
    const ACT_ES = { restart: "reiniciar", deploy: "redesplegar", rollback: "revertir" } as const;
    for (const m of metas) {
      for (const a of ["restart", "deploy", "rollback"] as const) {
        out.push({ kind: "action", label: `${ACT_ES[a]} ${m.name}`, hint: "acción", run: () => onAction(m.name, a) });
      }
    }
    const f = q.trim().toLowerCase();
    return f === "" ? out.slice(0, 9) : out.filter((t) => t.label.toLowerCase().includes(f)).slice(0, 12);
  }, [q, apps, metas, deps, goPage, openApp, openDeploy, onAction]);

  useEffect(() => setIdx(0), [q]);

  if (!open) return null;
  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-ink/80 px-4 pt-20 backdrop-blur-sm"
      onClick={() => setOpen(false)}
      role="dialog"
      aria-modal="true"
      aria-label="paleta de comandos"
    >
      <div
        className="w-full max-w-xl border border-border bg-popover shadow-2xl"
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown") {
            e.preventDefault();
            setIdx((i) => Math.min(i + 1, targets.length - 1));
          } else if (e.key === "ArrowUp") {
            e.preventDefault();
            setIdx((i) => Math.max(i - 1, 0));
          } else if (e.key === "Enter" && targets[idx]) {
            setOpen(false);
            targets[idx].run();
          } else if (e.key === "Escape") {
            setOpen(false);
          }
        }}
      >
        <div className="flex items-center gap-2 border-b border-border px-3">
          <Search className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="App, despliegue, acción…"
            className="h-10 min-w-0 flex-1 bg-transparent font-mono text-[12px] outline-none placeholder:text-muted-foreground/60"
          />
          <Kbd>esc</Kbd>
        </div>
        <div className="max-h-80 overflow-auto py-1">
          {targets.map((t, i) => (
            <button
              key={`${t.kind}-${t.label}`}
              onMouseEnter={() => setIdx(i)}
              onClick={() => {
                setOpen(false);
                t.run();
              }}
              className={`relative flex w-full items-center gap-2.5 px-3 py-2 text-left ${
                i === idx ? "bg-accent text-foreground" : "text-foreground/80"
              }`}
            >
              {i === idx && <span aria-hidden className="absolute top-1 bottom-1 left-0 w-px bg-signal" />}
              {t.kind === "app" && (
                <Dot status={t.hint === "failed" ? "failed" : t.hint === "running" ? "running" : "stopped"} />
              )}
              {t.kind === "deployment" && <Dot status={t.hint} />}
              <span className={`min-w-0 flex-1 truncate text-[12px] ${t.kind === "app" || t.kind === "deployment" ? "font-mono" : ""}`}>
                {t.label}
              </span>
              <span className="shrink-0 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase">
                {KIND_LABEL[t.kind]}
              </span>
            </button>
          ))}
          {targets.length === 0 && <p className="px-4 py-5 font-mono text-[10px] text-muted-foreground">sin resultados</p>}
        </div>
        <div className="flex items-center gap-3 border-t border-border px-3 py-1.5 font-mono text-[9px] text-muted-foreground">
          <span className="flex items-center gap-1"><Kbd>↑</Kbd><Kbd>↓</Kbd> navegar</span>
          <span className="flex items-center gap-1"><Kbd>↵</Kbd> abrir</span>
          <span className="flex items-center gap-1"><Kbd>esc</Kbd> cerrar</span>
          <span className="flex-1" />
          <span>{targets.length} objetivos</span>
        </div>
      </div>
    </div>
  );
}
