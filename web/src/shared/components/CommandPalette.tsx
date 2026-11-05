import { useEffect, useMemo, useRef, useState } from "react";
import { Search } from "lucide-react";
import { Deployment, AppMeta } from "../../shared/lib/types";
import { Dot, Kbd } from "../../shared/components/ui";
import { stateLabel } from "../lib/labels";
import { useI18n } from "../i18n";

export interface PaletteTarget {
  kind: "page" | "app" | "deployment" | "action";
  label: string;
  hint: string;
  run: () => void;
}

const KIND_LABEL: Record<PaletteTarget["kind"], string> = {
  page: "shared.palette.kind.page",
  app: "shared.palette.kind.app",
  deployment: "shared.palette.kind.deployment",
  action: "shared.palette.kind.action",
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
  const { t, tn } = useI18n();
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
      [t("nav.overview"), "overview"],
      [t("nav.deployments"), "deployments"],
      [t("nav.ci"), "ci"],
      [t("nav.logs"), "logs"],
      [t("nav.analytics"), "analytics"],
      [t("nav.observability"), "observability"],
      [t("nav.alerts"), "alerts"],
      [t("nav.routes"), "routes"],
      [t("nav.environment"), "environment"],
      [t("nav.settings"), "settings"],
      [t("nav.users"), "users"],
    ];
    for (const [label, p] of pages) {
      out.push({ kind: "page", label: t("shared.palette.goTo", { page: label }), hint: t("shared.palette.kind.page"), run: () => goPage(p) });
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
    const ACT: Record<"restart" | "deploy" | "rollback", string> = {
      restart: "shared.palette.action.restart",
      deploy: "shared.palette.action.deploy",
      rollback: "shared.palette.action.rollback",
    };
    for (const m of metas) {
      for (const a of ["restart", "deploy", "rollback"] as const) {
        out.push({ kind: "action", label: `${t(ACT[a])} ${m.name}`, hint: t("shared.palette.kind.action"), run: () => onAction(m.name, a) });
      }
    }
    const f = q.trim().toLowerCase();
    return f === "" ? out.slice(0, 9) : out.filter((x) => x.label.toLowerCase().includes(f)).slice(0, 12);
  }, [q, apps, metas, deps, goPage, openApp, openDeploy, onAction, t]);

  useEffect(() => setIdx(0), [q]);

  if (!open) return null;
  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-ink/80 px-4 pt-20 backdrop-blur-sm"
      onClick={() => setOpen(false)}
      role="dialog"
      aria-modal="true"
      aria-label={t("shared.palette.aria")}
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
            placeholder={t("shared.palette.placeholder")}
            className="h-10 min-w-0 flex-1 bg-transparent font-mono text-[12px] outline-none placeholder:text-muted-foreground/60"
          />
          <Kbd>esc</Kbd>
        </div>
        <div className="max-h-80 overflow-auto py-1">
          {targets.map((target, i) => (
            <button
              key={`${target.kind}-${target.label}`}
              onMouseEnter={() => setIdx(i)}
              onClick={() => {
                setOpen(false);
                target.run();
              }}
              className={`relative flex w-full items-center gap-2.5 px-3 py-2 text-left ${
                i === idx ? "bg-accent text-foreground" : "text-foreground/80"
              }`}
            >
              {i === idx && <span aria-hidden className="absolute top-1 bottom-1 left-0 w-px bg-signal" />}
              {target.kind === "app" && (
                <Dot status={target.hint === "failed" ? "failed" : target.hint === "running" ? "running" : "stopped"} />
              )}
              {target.kind === "deployment" && <Dot status={target.hint} />}
              <span className={`min-w-0 flex-1 truncate text-[12px] ${target.kind === "app" || target.kind === "deployment" ? "font-mono" : ""}`}>
                {target.label}
              </span>
              <span className="shrink-0 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase">
                {t(KIND_LABEL[target.kind])}
              </span>
            </button>
          ))}
          {targets.length === 0 && <p className="px-4 py-5 font-mono text-[10px] text-muted-foreground">{t("shared.palette.empty")}</p>}
        </div>
        <div className="flex items-center gap-3 border-t border-border px-3 py-1.5 font-mono text-[9px] text-muted-foreground">
          <span className="flex items-center gap-1"><Kbd>↑</Kbd><Kbd>↓</Kbd> {t("shared.palette.navigate")}</span>
          <span className="flex items-center gap-1"><Kbd>↵</Kbd> {t("shared.palette.open")}</span>
          <span className="flex items-center gap-1"><Kbd>esc</Kbd> {t("shared.palette.close")}</span>
          <span className="flex-1" />
          <span>{tn("shared.palette.targets", targets.length)}</span>
        </div>
      </div>
    </div>
  );
}
