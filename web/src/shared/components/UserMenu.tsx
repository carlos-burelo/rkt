import { Monitor, Moon, Sun } from "lucide-react";
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent,
  DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { Avatar } from "./ui";
import { roleLabel } from "../lib/labels";
import { useAuth } from "../../features/auth/store";
import { useTheme, type Theme } from "../theme";
import { useI18n } from "../i18n";

const THEME_ICONS: Record<Theme, typeof Sun> = { system: Monitor, light: Sun, dark: Moon };

// Menú de cuenta en el header: identidad, tema y salida. Reemplaza el
// bloque de cuenta + toggle que vivía en el footer del sidebar.
export function UserMenu() {
  const { t } = useI18n();
  const { who, doLogout } = useAuth();
  const { theme, setTheme } = useTheme();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="flex h-7 w-7 items-center justify-center rounded-full outline-none hover:ring-2 hover:ring-signal/50"
        aria-label={who?.user ?? t("shell.account")}
      >
        <Avatar name={who?.user ?? "?"} className="h-6 w-6" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <div className="flex items-center gap-2 px-2 py-2">
          <Avatar name={who?.user ?? "?"} />
          <div className="min-w-0 flex-1 leading-tight">
            <p className="truncate text-[12px] font-medium">{who?.user ?? "…"}</p>
            <p className="tnum truncate font-mono text-[10px] text-muted-foreground">{roleLabel(who?.role ?? "")}</p>
          </div>
        </div>
        <DropdownMenuLabel className="micro">{t("theme.label")}</DropdownMenuLabel>
        <DropdownMenuRadioGroup
          value={theme}
          onValueChange={(v) => setTheme(v as Theme)}
        >
          {(["system", "light", "dark"] as const).map((th) => {
            const Icon = THEME_ICONS[th];
            return (
              <DropdownMenuRadioItem key={th} value={th} className="font-mono text-[11px]">
                <Icon className="h-3.5 w-3.5" aria-hidden /> {t(`theme.${th}`)}
              </DropdownMenuRadioItem>
            );
          })}
        </DropdownMenuRadioGroup>
        <DropdownMenuItem onClick={() => doLogout()} className="font-mono text-[11px]">
          {t("shell.logout")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
