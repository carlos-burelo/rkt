import { useNavigate } from "react-router";
import { ArrowLeft, Database } from "lucide-react";
import {
  SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarMenu, SidebarMenuButton, SidebarMenuItem,
} from "@/components/ui/sidebar";
import { useI18n } from "../../shared/i18n";
import { useAuth } from "../auth/store";
import { useDb, type DbTab } from "./store";
import { TableTree } from "./TableTree";
import { ProvisionDialog } from "./TableDialogs";

// Sidebar contextual del módulo (mismo mecanismo que AppSidebar en
// /app/*): cuando estás en /databases el sidebar cambia y muestra el
// explorador de la base en vez de la navegación global. Volver atrás
// restaura el nav global.
const TABS: { id: DbTab; label: string }[] = [
  { id: "tablas", label: "db.tables" },
  { id: "esquema", label: "db.schemaTab" },
  { id: "objetos", label: "db.objects" },
  { id: "sql", label: "db.sql" },
  { id: "copias", label: "db.backups" },
];

export function DbSidebar() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const canWrite = useAuth((a) => a.canWrite);
  const apps = useDb((s) => s.apps);
  const sel = useDb((s) => s.sel);
  const tables = useDb((s) => s.tables);
  const table = useDb((s) => s.table);
  const tab = useDb((s) => s.tab);
  const setTab = useDb((s) => s.setTab);
  const selectApp = useDb((s) => s.selectApp);
  const selectTable = useDb((s) => s.selectTable);
  const refreshTables = useDb((s) => s.refreshTables);
  const loadApps = useDb((s) => s.loadApps);

  const back = () => navigate("/overview");

  return (
    <>
      <button
        type="button"
        onClick={back}
        className="flex h-8 w-full items-center gap-2 border border-border bg-card/40 px-2 text-left font-mono text-[11px] text-muted-foreground transition-colors hover:border-input hover:text-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
        aria-label={t("common.back")}
      >
        <ArrowLeft className="h-3.5 w-3.5 shrink-0" />
        <span className="truncate group-data-[collapsible=icon]:hidden">{t("common.back")}</span>
      </button>
      <div className="mt-3 mb-1 flex items-center gap-2 px-2 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0">
        <span className="flex h-7 w-7 shrink-0 items-center justify-center border border-signal/40 bg-signal/10 text-signal">
          <Database className="h-3.5 w-3.5" />
        </span>
        <div className="min-w-0 flex-1 leading-tight group-data-[collapsible=icon]:hidden">
          <p className="truncate text-[12px] font-medium">{t("nav.databases")}</p>
          <p className="tnum truncate font-mono text-[9px] text-muted-foreground">
            {sel ? `${sel}${table ? ` · ${table}` : ""}` : t("db.pickApp")}
          </p>
        </div>
      </div>

      <div className="mt-2 px-2 group-data-[collapsible=icon]:hidden">
        <label className="micro mb-1 block text-sidebar-foreground/40" htmlFor="db-base">
          {t("db.baseLabel")}
        </label>
        <select
          id="db-base"
          value={sel}
          onChange={(e) => selectApp(e.target.value)}
          className="h-8 w-full border border-input bg-field/35 px-1.5 font-mono text-[12px]"
        >
          {apps.map((a) => (
            <option key={a.app} value={a.app}>
              {a.app} · {a.mode}
            </option>
          ))}
        </select>
      </div>

      <nav aria-label={t("nav.databases")} className="mt-1 group-data-[collapsible=icon]:hidden">
        <SidebarGroup className="py-0">
          <SidebarGroupLabel className="mt-3 mb-1 flex h-4 items-center gap-2 px-2 font-mono text-[9px] tracking-[0.18em] uppercase text-sidebar-foreground/40">
            <span>{t("shell.dbSection")}</span>
            <span aria-hidden className="h-px flex-1 bg-sidebar-border" />
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu className="gap-0.5">
              {TABS.map((tb) => (
                <SidebarMenuItem key={tb.id}>
                  <SidebarMenuButton
                    isActive={tab === tb.id}
                    onClick={() => setTab(tb.id)}
                    tooltip={t(tb.label)}
                    className="relative h-8 rounded-[1px] px-2 text-[12px] font-normal text-sidebar-foreground/65 hover:bg-sidebar-accent hover:text-sidebar-foreground data-active:bg-sidebar-accent data-active:font-medium data-active:text-sidebar-foreground before:absolute before:top-1.5 before:bottom-1.5 before:left-0 before:w-px before:bg-signal before:opacity-0 data-active:before:opacity-100"
                  >
                    <span className="flex-1 truncate text-left">{t(tb.label)}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup className="py-0">
          <SidebarGroupLabel className="mt-3 mb-1 flex h-4 items-center gap-2 px-2 font-mono text-[9px] tracking-[0.18em] uppercase text-sidebar-foreground/40">
            <span>{t("db.explorer")}</span>
            <span aria-hidden className="h-px flex-1 bg-sidebar-border" />
            <span className="tnum text-[9px] text-sidebar-foreground/55">{tables.length}</span>
          </SidebarGroupLabel>
          <SidebarGroupContent>
            {sel ? (
              <TableTree
                app={sel}
                tables={tables}
                current={table}
                onSelect={(name) => {
                  selectTable(name);
                  setTab("tablas");
                }}
                onCreated={() => void refreshTables()}
              />
            ) : (
              <p className="px-2 py-2 font-mono text-[11px] text-muted-foreground">{t("db.pickApp")}</p>
            )}
          </SidebarGroupContent>
        </SidebarGroup>
      </nav>

      {canWrite && (
        <div className="mt-3 px-2 group-data-[collapsible=icon]:hidden">
          <ProvisionDialog onCreated={(app) => void loadApps(app)} />
        </div>
      )}
    </>
  );
}
