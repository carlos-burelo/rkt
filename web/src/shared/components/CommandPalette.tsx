import { useEffect, useMemo, useRef, useState } from "react";
import { Deployment, AppMeta } from "../../shared/lib/types";
import { Dot } from "../../shared/components/ui";

export interface PaletteTarget {
  kind: "page" | "app" | "deployment" | "action";
  label: string;
  hint: string;
  run: () => void;
}

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
  goPage: (p: "overview" | "deployments" | "logs" | "routes" | "settings" | "users" | "analytics" | "observability" | "environment" | "ci") => void;
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
  }, [open ]);

  const targets: PaletteTarget[] = useMemo(() => {
    const out: PaletteTarget[] = [];
    const pages: [string, "overview" | "deployments" | "logs" | "routes" | "settings" | "users" | "analytics" | "observability" | "environment" | "ci"][] = [
      ["Overview", "overview"],
      ["Deployments", "deployments"],
      ["CI", "ci"],
      ["Logs", "logs"],
      ["Analytics", "analytics"],
      ["Observability", "observability"],
      ["Domains", "routes"],
      ["Environment", "environment"],
      ["Settings", "settings"],
      ["Users", "users"],
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
    for (const m of metas) {
      for (const a of ["restart", "deploy", "rollback"] as const) {
        out.push({ kind: "action", label: `${a} ${m.name}`, hint: "acción", run: () => onAction(m.name, a) });
      }
    }
    const f = q.trim().toLowerCase();
    const filtered = f === "" ? out.slice(0, 9) : out.filter((t) => t.label.toLowerCase().includes(f)).slice(0, 12);
    return filtered;
  }, [q, apps, metas, deps, goPage, openApp, openDeploy, onAction]);

  useEffect(() => setIdx(0), [q]);

  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/70 pt-24" onClick={() => setOpen(false)}>
      <div
        className="w-full max-w-lg overflow-hidden rounded-lg border border-zinc-700 bg-zinc-900 shadow-2xl"
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
        <input
          ref={inputRef}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="App, deployment, acción…"
          className="w-full border-b border-zinc-800 bg-transparent px-4 py-3 text-sm outline-none placeholder:text-zinc-600"
        />
        <div className="max-h-72 overflow-auto p-1.5">
          {targets.map((t, i) => (
            <button
              key={`${t.kind}-${t.label}`}
              onMouseEnter={() => setIdx(i)}
              onClick={() => {
                setOpen(false);
                t.run();
              }}
              className={`flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-left text-sm ${
                i === idx ? "bg-zinc-700/60" : ""
              }`}
            >
              {t.kind === "app" && (
                <Dot status={t.hint === "failed" ? "failed" : t.hint === "running" ? "running" : "stopped"} />
              )}
              {t.kind === "deployment" && <Dot status={t.hint} />}
              <span className={t.kind === "app" || t.kind === "deployment" ? "font-mono" : ""}>{t.label}</span>
              <span className="flex-1" />
              <span className="text-[11px] text-zinc-500">{t.hint}</span>
            </button>
          ))}
          {targets.length === 0 && <p className="p-4 text-sm text-zinc-500">Sin resultados.</p>}
        </div>
        <p className="border-t border-zinc-800 px-4 py-1.5 font-mono text-[11px] text-zinc-600">↑↓ navegar · enter abrir · esc cerrar</p>
      </div>
    </div>
  );
}
