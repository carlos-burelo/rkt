import type { ReactNode } from "react";
import { FolderGit2, FolderOpen, Laptop, Terminal } from "lucide-react";

export type Origin = "git" | "local" | "external" | "manual";
export type Step = 1 | 2 | 3;

export const ORIGINS: { id: Origin; labelKey: string; hintKey: string; icon: typeof FolderGit2 }[] = [
  { id: "git", labelKey: "create.origin.git", hintKey: "create.origin.gitHint", icon: FolderGit2 },
  { id: "local", labelKey: "create.origin.local", hintKey: "create.origin.localHint", icon: FolderOpen },
  { id: "external", labelKey: "create.origin.external", hintKey: "create.origin.externalHint", icon: Laptop },
  { id: "manual", labelKey: "create.origin.manual", hintKey: "create.origin.manualHint", icon: Terminal },
];

export const SLUG = /^[a-z0-9][a-z0-9-]*$/;

// slug desde la última parte del repo (git o ruta local).
export function slugOf(src: string): string {
  const clean = src.trim().replace(/[\\/]+$/, "").replace(/\.git$/i, "");
  const base = clean.split(/[\\/]/).pop() ?? "";
  return base
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40);
}

export function looksGit(src: string): boolean {
  return /^(https?:\/\/|git@|ssh:\/\/)/i.test(src.trim());
}

export function Field({
  label,
  hint,
  error,
  children,
}: {
  label: string;
  hint?: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <label className="field-label">
      <span className="flex items-baseline justify-between gap-2">
        {label}
        {hint && <span className="font-mono text-[9px] text-muted-foreground/70 normal-case">{hint}</span>}
      </span>
      {children}
      {error && <span className="font-mono text-[10px] normal-case text-bad">{error}</span>}
    </label>
  );
}
