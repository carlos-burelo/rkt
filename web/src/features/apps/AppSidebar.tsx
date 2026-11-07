import { NavLink, useNavigate } from "react-router";
import { ArrowLeft } from "lucide-react";
import { Avatar, Dot } from "../../shared/components/ui";
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";

// Navegación exclusiva de una app: Resumen, Uso, Despliegues, Registros,
// Analítica, Entorno, Ajustes. Nada global aquí salvo regresar.
export const APP_TABS = [
  { id: "overview", label: "nav.overview" },
  { id: "usage", label: "apps.usage.title" },
  { id: "deploys", label: "nav.deployments" },
  { id: "logs", label: "nav.logs" },
  { id: "analytics", label: "nav.analytics" },
  { id: "env", label: "nav.environment" },
  { id: "settings", label: "nav.settings" },
] as const;

export function AppSidebar({ name }: { name: string }) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const { status, metas } = useFleet();
  const setAppFilter = useUI((u) => u.setAppFilter);
  const meta = metas.find((m) => m.name === name);
  const st = status?.apps.find((a) => a.name === name);
  const state = meta?.disabled ? "stopped" : st?.failed ? "failed" : st?.running ? (st.http_ok === false ? "degraded" : "running") : "stopped";
  const tabs = APP_TABS.filter((tab) => !(meta?.external && tab.id === "deploys"));

  const back = () => {
    setAppFilter("");
    navigate("/overview");
  };

  return (
    <>
      <button
        type="button"
        onClick={back}
        className="flex h-8 w-full items-center gap-2 border border-border bg-card/40 px-2 text-left font-mono text-[11px] text-muted-foreground transition-colors hover:border-input hover:text-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
        aria-label={t("common.back")}
      >
        <ArrowLeft className="h-3.5 w-3.5 shrink-0" />
        <span className="truncate group-data-[collapsible=icon]:hidden">{t("shell.backToApps")}</span>
      </button>
      <div className="mt-3 mb-1 flex items-center gap-2 px-2 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0">
        <Avatar name={name} />
        <div className="min-w-0 flex-1 leading-tight group-data-[collapsible=icon]:hidden">
          <p className="truncate font-mono text-[12px] font-medium">{name}</p>
          <p className="tnum flex items-center gap-1 font-mono text-[9px] text-muted-foreground">
            <Dot status={state} />
            {state}
          </p>
        </div>
      </div>
      <nav aria-label={name} className="mt-1">
        <SidebarMenu className="gap-0.5">
          {tabs.map((tab) => (
            <SidebarMenuItem key={tab.id}>
              <NavLink to={`/app/${encodeURIComponent(name)}/${tab.id}`} className="w-full">
                {({ isActive }) => (
                  <SidebarMenuButton
                    isActive={isActive}
                    tooltip={t(tab.label)}
                    className="h-8 rounded-[1px] px-2 text-[12px] font-normal text-sidebar-foreground/65 hover:bg-sidebar-accent hover:text-sidebar-foreground data-active:bg-sidebar-accent data-active:font-medium data-active:text-sidebar-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0 [&>svg]:size-3.5"
                  >
                    <span className="flex-1 truncate text-left group-data-[collapsible=icon]:hidden">{t(tab.label)}</span>
                  </SidebarMenuButton>
                )}
              </NavLink>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </nav>
    </>
  );
}
