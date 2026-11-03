import { useEffect } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router";
import { toast } from "sonner";
import { Toaster } from "@/components/ui/sonner";
import {
  LayoutGrid, Network, Rocket, Hammer, Terminal, BarChart3, Activity,
  Bell, Globe, FlaskConical, Settings as SettingsIcon, Users as UsersIcon,
  Plus, Search, ChevronRight, Command,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent,
  SidebarGroupLabel, SidebarHeader, SidebarInset, SidebarMenu,
  SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarRail, SidebarTrigger,
} from "@/components/ui/sidebar";
import { Avatar, BrandMark, Dot, Err, Kbd, stateOf } from "../shared/components/ui";
import { roleLabel } from "../shared/lib/labels";
import { InstallButton } from "../shared/components/InstallButton";
import { CommandPalette } from "../shared/components/CommandPalette";
import { useAuth } from "../features/auth/store";
import { useUI } from "./ui";
import { useFleet } from "../features/overview/hooks";
import { CreateDialog } from "../features/apps/CreateDialog";
import { DeployDrawer } from "../features/deployments/DeployDrawer";

const GROUPS = [
  {
    label: "Operar",
    items: [
      { to: "/overview", label: "Resumen", icon: LayoutGrid },
      { to: "/deployments", label: "Despliegues", icon: Rocket },
      { to: "/ci", label: "CI", icon: Hammer },
      { to: "/logs", label: "Registros", icon: Terminal },
    ],
  },
  {
    label: "Servir",
    items: [
      { to: "/routes", label: "Dominios", icon: Globe },
      { to: "/proxy", label: "Proxy", icon: Network },
      { to: "/environment", label: "Entorno", icon: FlaskConical },
    ],
  },
  {
    label: "Observar",
    items: [
      { to: "/analytics", label: "Analítica", icon: BarChart3 },
      { to: "/observability", label: "Observabilidad", icon: Activity },
      { to: "/alerts", label: "Alertas", icon: Bell },
    ],
  },
  {
    label: "Sistema",
    items: [
      { to: "/settings", label: "Ajustes", icon: SettingsIcon },
      { to: "/users", label: "Usuarios", icon: UsersIcon, admin: true },
    ],
  },
] as const;

const PAGE_LABEL: Record<string, string> = {
  overview: "Resumen",
  deployments: "Despliegues",
  ci: "CI",
  logs: "Registros",
  routes: "Dominios",
  proxy: "Proxy",
  environment: "Entorno",
  analytics: "Analítica",
  observability: "Observabilidad",
  alerts: "Alertas",
  settings: "Ajustes",
  users: "Usuarios",
  app: "App",
};

export function ShellLayout() {
  const { who, isAdmin, canWrite, brand, doLogout } = useAuth();
  const online = useUI((u) => u.online);
  const setOnline = useUI((u) => u.setOnline);
  const palette = useUI((u) => u.palette);
  const setPalette = useUI((u) => u.setPalette);
  const setCreateOpen = useUI((u) => u.setCreateOpen);
  const appFilter = useUI((u) => u.appFilter);
  const setAppFilter = useUI((u) => u.setAppFilter);
  const err = useUI((u) => u.err);
  const { status, deps, metas, loading } = useFleet();
  const location = useLocation();
  const navigate = useNavigate();
  const crumbs = location.pathname.split("/").filter(Boolean);
  const title = PAGE_LABEL[crumbs[0] ?? "overview"] ?? "Resumen";
  const apps = status?.apps ?? [];
  const running = apps.filter((a) => a.running && !a.failed).length;
  const failed = apps.filter((a) => a.failed).length;
  const runningDeps = deps.filter((d) => d.status === "running").length;
  const currentApp = location.pathname.startsWith("/app/") ? decodeURIComponent(crumbs[1] ?? "") : "";

  useEffect(() => {
    document.title = `rkt · ${title}`;
  }, [title]);

  useEffect(() => {
    setOnline(navigator.onLine);
    const on = () => setOnline(true);
    const off = () => setOnline(false);
    window.addEventListener("online", on);
    window.addEventListener("offline", off);
    return () => {
      window.removeEventListener("online", on);
      window.removeEventListener("offline", off);
    };
  }, [setOnline]);

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPalette(!useUI.getState().palette);
      } else if (
        e.key === "/" &&
        !(e.target instanceof HTMLInputElement) &&
        !(e.target instanceof HTMLTextAreaElement) &&
        !(e.target instanceof HTMLSelectElement)
      ) {
        e.preventDefault();
        setPalette(true);
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [setPalette]);

  function openApp(name: string) {
    setAppFilter(name);
    navigate(`/app/${name}`);
  }

  return (
    <SidebarProvider>
      <Toaster theme="dark" position="bottom-right" />
      <Sidebar collapsible="icon" className="border-r border-sidebar-border">
        <SidebarHeader className="gap-0 border-b border-sidebar-border px-3 py-3 group-data-[collapsible=icon]:px-2">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                size="lg"
                onClick={() => navigate("/overview")}
                className="h-auto gap-2.5 rounded-none p-0 hover:bg-transparent group-data-[collapsible=icon]:justify-center"
              >
                <span className="flex h-8 w-8 shrink-0 items-center justify-center border border-signal/40 bg-signal/10 text-signal">
                  <BrandMark className="h-4 w-4" />
                </span>
                <div className="grid min-w-0 flex-1 text-left leading-tight group-data-[collapsible=icon]:hidden">
                  <span className="flex items-baseline gap-1.5">
                    <span className="truncate text-[14px] font-semibold tracking-tight">{brand}</span>
                    <span className="micro">terminal</span>
                  </span>
                  <span className="tnum truncate font-mono text-[10px] text-muted-foreground">
                    {apps.length === 0 ? "terminal vacía" : `${running}/${apps.length} en ruta${failed > 0 ? ` · ${failed} varado${failed > 1 ? "s" : ""}` : ""}`}
                  </span>
                </div>
                <span className="micro tnum hidden shrink-0 text-muted-foreground/60 group-data-[collapsible=icon]:hidden xl:inline">
                  v{status?.version ?? "…"}
                </span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>

          {canWrite && (
            <Button className="mt-3 w-full justify-between px-2.5 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0" size="sm" onClick={() => setCreateOpen(true)}>
              <span className="inline-flex items-center gap-1.5">
                <Plus className="h-3.5 w-3.5" />
                <span className="group-data-[collapsible=icon]:hidden">Nueva app</span>
              </span>
              <span className="group-data-[collapsible=icon]:hidden">
                <Kbd>N</Kbd>
              </span>
            </Button>
          )}

          <button
            onClick={() => setPalette(true)}
            className="mt-2 flex h-8 w-full items-center gap-2 border border-border bg-field/25 px-2 text-left font-mono text-[10px] text-muted-foreground transition-colors hover:border-input hover:text-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
          >
            <Search className="h-3.5 w-3.5 shrink-0" />
            <span className="flex-1 truncate group-data-[collapsible=icon]:hidden">Buscar o accionar</span>
            <span className="flex items-center gap-0.5 group-data-[collapsible=icon]:hidden">
              <Kbd>⌘</Kbd><Kbd>K</Kbd>
            </span>
          </button>
        </SidebarHeader>

        <SidebarContent className="gap-0 px-2 pb-2">
          {GROUPS.map((g) => {
            const items = g.items.filter((n) => !("admin" in n && n.admin) || isAdmin);
            if (items.length === 0) return null;
            return (
              <SidebarGroup key={g.label} className="py-0 group-data-[collapsible=icon]:p-0">
                <SidebarGroupLabel className="mt-4 mb-1 flex h-4 items-center gap-2 px-2 font-mono text-[9px] tracking-[0.18em] uppercase text-sidebar-foreground/40 group-data-[collapsible=icon]:hidden">
                  <span>{g.label}</span>
                  <span aria-hidden className="h-px flex-1 bg-sidebar-border" />
                </SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu className="gap-0.5">
                    {items.map((n) => (
                      <SidebarMenuItem key={n.to}>
                        <NavLink to={n.to} className="w-full">
                          {({ isActive }) => (
                            <SidebarMenuButton
                              isActive={isActive}
                              tooltip={n.label}
                              className="relative h-8 rounded-[1px] px-2 text-[12px] font-normal text-sidebar-foreground/65 hover:bg-sidebar-accent hover:text-sidebar-foreground data-active:bg-sidebar-accent data-active:font-medium data-active:text-sidebar-foreground before:absolute before:top-1.5 before:bottom-1.5 before:left-0 before:w-px before:bg-signal before:opacity-0 data-active:before:opacity-100 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0 [&>svg]:size-3.5 [&>svg]:text-muted-foreground/70 data-active:[&>svg]:text-signal"
                            >
                              <n.icon />
                              <span className="flex-1 group-data-[collapsible=icon]:hidden">{n.label}</span>
                              {n.to === "/deployments" && runningDeps > 0 && (
                                <span className="tnum font-mono text-[10px] text-signal">{runningDeps}</span>
                              )}
                              {n.to === "/alerts" && failed > 0 && (
                                <span className="tnum font-mono text-[10px] text-bad">{failed}</span>
                              )}
                            </SidebarMenuButton>
                          )}
                        </NavLink>
                      </SidebarMenuItem>
                    ))}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            );
          })}

          <SidebarGroup className="mt-4 py-0 group-data-[collapsible=icon]:p-0">
            <SidebarGroupLabel className="mb-1 flex h-4 items-center gap-2 px-2 font-mono text-[9px] tracking-[0.18em] uppercase text-sidebar-foreground/40 group-data-[collapsible=icon]:hidden">
              <span>Bahías</span>
              <span aria-hidden className="h-px flex-1 bg-sidebar-border" />
              <span className="tnum text-[9px] text-sidebar-foreground/55">{apps.length}</span>
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu className="gap-0.5">
                <SidebarMenuItem>
                  <SidebarMenuButton
                    isActive={appFilter === ""}
                    onClick={() => setAppFilter("")}
                    tooltip="todas las bahías"
                    className="h-7 rounded-[1px] px-2 font-mono text-[10px] text-sidebar-foreground/55 hover:bg-sidebar-accent hover:text-sidebar-foreground data-active:bg-sidebar-accent data-active:text-sidebar-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
                  >
                    <span className="hidden h-3.5 w-3.5 shrink-0 items-center justify-center text-muted-foreground/60 group-data-[collapsible=icon]:flex">
                      <LayoutGrid className="h-3.5 w-3.5" />
                    </span>
                    <span className="flex-1 group-data-[collapsible=icon]:hidden">todas</span>
                    <span className="tnum text-[9px] text-muted-foreground/60 group-data-[collapsible=icon]:hidden">{apps.length}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                {apps.map((a) => {
                  const st = stateOf(a);
                  const meta = metas.find((m) => m.name === a.name);
                  const domain = meta?.domains[0];
                  return (
                    <SidebarMenuItem key={a.name}>
                      <SidebarMenuButton
                        isActive={appFilter === a.name}
                        onClick={() => openApp(a.name)}
                        className="h-7 rounded-[1px] px-2 font-mono text-[10px] text-sidebar-foreground/60 hover:bg-sidebar-accent hover:text-sidebar-foreground data-active:bg-sidebar-accent data-active:text-sidebar-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
                        tooltip={domain ? `${a.name} → ${domain}` : a.name}
                      >
                        <Dot status={st} />
                        <span className="min-w-0 flex-1 truncate group-data-[collapsible=icon]:hidden">{a.name}</span>
                        {meta?.port && (
                          <span className="tnum text-[9px] text-muted-foreground/55 group-data-[collapsible=icon]:hidden">:{meta.port}</span>
                        )}
                        {st === "failed" && (
                          <span className="tnum text-[9px] text-bad group-data-[collapsible=icon]:hidden">!{a.restarts}</span>
                        )}
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  );
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        <SidebarFooter className="gap-2 border-t border-sidebar-border px-3 py-3 group-data-[collapsible=icon]:px-2">
          {!online && (
            <p className="border border-warn/30 bg-warn/8 px-2 py-1.5 font-mono text-[10px] text-warn group-data-[collapsible=icon]:hidden">
              sin conexión · lectura local
            </p>
          )}
          <div className="group-data-[collapsible=icon]:hidden">
            <InstallButton />
          </div>
          <SidebarMenu>
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger className="flex w-full items-center gap-2 rounded-[1px] px-1 py-1 text-left outline-none hover:bg-sidebar-accent group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0">
                  <Avatar name={who?.user ?? "?"} />
                  <div className="grid min-w-0 flex-1 text-left leading-tight group-data-[collapsible=icon]:hidden">
                    <span className="truncate text-[11px] font-medium">{who?.user}</span>
                    <span className="tnum truncate font-mono text-[9px] text-muted-foreground">
                      {roleLabel(who?.role ?? "")} · v{status?.version ?? "…"}
                    </span>
                  </div>
                  <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground/50 group-data-[collapsible=icon]:hidden" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-40">
                  <DropdownMenuItem onClick={() => doLogout()}>
                    Salir
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>

      <SidebarInset className="min-w-0 bg-transparent">
        <header className="sticky top-0 z-20 flex h-12 items-center gap-2 border-b border-border bg-background/92 px-3 backdrop-blur-md md:px-4">
          <SidebarTrigger className="h-7 w-7 text-muted-foreground hover:bg-accent hover:text-foreground" />
          <span aria-hidden className="hidden h-3 w-px bg-border sm:block" />
          <nav aria-label="ruta" className="flex min-w-0 items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
            <span className="shrink-0 tracking-[0.08em] text-foreground/60">{brand}</span>
            <span aria-hidden className="text-muted-foreground/35">/</span>
            <span className="truncate text-foreground">{title}</span>
            {currentApp && (
              <>
                <span aria-hidden className="text-muted-foreground/35">/</span>
                <span className="tnum truncate text-foreground/70">{currentApp}</span>
              </>
            )}
            {!currentApp && crumbs[1] && (
              <>
                <span aria-hidden className="text-muted-foreground/35">/</span>
                <span className="tnum truncate text-foreground/70">{decodeURIComponent(crumbs[1])}</span>
              </>
            )}
          </nav>
          <span className="flex-1" />
          <span className="folio hidden items-center gap-1.5 lg:flex">
            <Command className="h-3 w-3" />
            {new Date().toISOString().slice(0, 10)}
          </span>
          {!online && <span className="font-mono text-[10px] text-warn">offline</span>}
          <span className="hidden items-center gap-1.5 border border-border bg-card/40 px-2 py-1 md:flex">
            <Dot status={failed > 0 ? "failed" : "ok"} />
            <span className="tnum font-mono text-[10px] text-muted-foreground">
              {failed > 0 ? `${failed} varado${failed > 1 ? "s" : ""}` : `${running}/${apps.length} en ruta`}
            </span>
          </span>
          {canWrite && (
            <Button size="sm" onClick={() => setCreateOpen(true)} className="h-7">
              <Plus className="h-3.5 w-3.5" /> Nueva app
            </Button>
          )}
          <Avatar name={who?.user ?? "?"} className="h-6 w-6" />
        </header>
        <main className="min-w-0 flex-1 px-3 py-5 md:px-5 md:py-6">
          <div className="mx-auto max-w-[1480px]">
            <Err msg={err} />
            {loading ? (
              <div className="space-y-px border border-border" aria-label="cargando flota">
                {Array.from({ length: 6 }).map((_, i) => (
                  <div key={i} className="flex h-14 items-center gap-3 border-b border-border bg-card/35 px-3 last:border-0">
                    <Skeleton className="h-3 w-3" />
                    <Skeleton className="h-3 w-32" />
                    <Skeleton className="h-3 w-20" />
                    <span className="flex-1" />
                    <Skeleton className="h-3 w-16" />
                  </div>
                ))}
              </div>
            ) : (
              <Outlet />
            )}
          </div>
        </main>
      </SidebarInset>

      <CreateDialogShell navigate={navigate} openApp={openApp} />
      <DeployDrawer />
      <CommandPaletteShell navigate={navigate} openApp={openApp} />
    </SidebarProvider>
  );
}

function CreateDialogShell({ navigate, openApp }: { navigate: (p: string) => void; openApp: (n: string) => void }) {
  const createOpen = useUI((u) => u.createOpen);
  const setCreateOpen = useUI((u) => u.setCreateOpen);
  const fail = useUI((u) => u.fail);
  return (
    <CreateDialog
      open={createOpen}
      setOpen={setCreateOpen}
      onCreated={(name) => {
        toast.success(`app ${name} creada`);
        openApp(name);
      }}
      goDomains={() => navigate("/routes")}
      fail={fail}
    />
  );
}

function CommandPaletteShell({ navigate, openApp }: { navigate: (p: string) => void; openApp: (n: string) => void }) {
  const palette = useUI((u) => u.palette);
  const setPalette = useUI((u) => u.setPalette);
  const setSel = useUI((u) => u.setSel);
  const act = useUI((u) => u.act);
  const { status, deps, metas } = useFleet();
  return (
    <CommandPalette
      open={palette}
      setOpen={setPalette}
      apps={status?.apps ?? []}
      metas={metas}
      deps={deps}
      goPage={(p) => navigate(`/${p}`)}
      openApp={openApp}
      openDeploy={setSel}
      onAction={act}
    />
  );
}
