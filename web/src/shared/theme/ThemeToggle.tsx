import { Monitor, Moon, Sun } from "lucide-react";
import { cn } from "@/lib/utils";
import { useI18n } from "../i18n";
import { useTheme, type Theme } from "./index";

const ICONS: Record<Theme, typeof Sun> = { system: Monitor, light: Sun, dark: Moon };

export function ThemeToggle({ compact = false, className }: { compact?: boolean; className?: string }) {
  const { t } = useI18n();
  const { theme, cycle } = useTheme();
  const Icon = ICONS[theme];
  const label = t(`theme.${theme}`);
  const aria = t("theme.toggle", { current: label });

  if (compact) {
    return (
      <button
        type="button"
        onClick={cycle}
        aria-label={aria}
        title={aria}
        className={cn(
          "flex h-8 w-8 items-center justify-center border border-border bg-card/40 text-muted-foreground transition-colors hover:border-input hover:text-foreground",
          className,
        )}
      >
        <Icon className="h-3.5 w-3.5" />
      </button>
    );
  }

  return (
    <button
      type="button"
      onClick={cycle}
      aria-label={aria}
      title={aria}
      className={cn(
        "flex h-7 w-full items-center gap-2 border border-transparent px-1 font-mono text-[10px] text-muted-foreground transition-colors hover:border-border hover:bg-accent hover:text-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0",
        className,
      )}
    >
      <Icon className="h-3.5 w-3.5 shrink-0" />
      <span className="truncate group-data-[collapsible=icon]:hidden">
        {t("theme.label")}: {label}
      </span>
    </button>
  );
}
