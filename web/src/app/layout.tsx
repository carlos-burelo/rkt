import { useEffect } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router";
import { toast } from "sonner";
import { Toaster } from "@/components/ui/sonner";
import {
  LayoutGrid, Network, Rocket, Hammer, Terminal, BarChart3, Activity,
  Bell, Globe, FlaskConical, Settings as SettingsIcon, Users as UsersIcon,
  Plus, Search,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent,
  SidebarGroupLabel, SidebarHeader, SidebarInset, SidebarMenu,
  SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarRail, SidebarTrigger,
  SidebarSeparator,
} from "@/components/ui/sidebar";
import { Dot, Err, stateOf } from "../shared/components/ui";
import { InstallButton } from "../shared/components/InstallButton";
import { CommandPalette } from "../shared/components/CommandPalette";
import { useAuth } from "../features/auth/store";
import { useUI } from "./ui";
import { useFleet } from "../features/overview/hooks";
import { CreateDialog } from "../features/apps/CreateDialog";
import { DeployDrawer } from "../features/deployments/DeployDrawer";

// Nav agrupada por trabajo PaaS: operar la flota, servirla al mundo,
// observarla y administrar el sistema. El orden sigue la frecuencia.
const GROUPS = [
  {
    label: "Operar",
    items: [
      { to: "/overview", label: "Overview", icon: LayoutGrid },
      { to: "/deployments", label: "Deployments", icon: Rocket },
      { to: "/ci", label: "CI", icon: Hammer },
      { to: "/logs", label: "Logs", icon: Terminal },
    ],
  },
  {
    label: "Servir",
    items: [
      { to: "/routes", label: "Domains", icon: Globe },
      { to: "/proxy", label: "Proxy", icon: Network },
      { to: "/environment", label: "Environment", icon: FlaskConical },
    ],
  },
  {
    label: "Observar",
    items: [
      { to: "/analytics", label: "Analytics", icon: BarChart3 },
      { to: "/observability", label: "Observability", icon: Activity },
      { to: "/alerts", label: "Alerts", icon: Bell },
    ],
  },
  {
    label: "Sistema",
    items: [
      { to: "/settings", label: "Settings", icon: SettingsIcon },
      { to: "/users", label: "Users", icon: UsersIcon, admin: true },
    ],
  },
] as const;

// ShellLayout: terminal portuaria. Rail tinta con manifiesto de la flota
// (apps = contenedores con bahía y destino) + <Outlet/> por ruta.
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
  const title = crumbs[0] ?? "overview";
  const apps = status?.apps ?? [];
  const running = apps.filter((a) => a.running && !a.failed).length;
  const failed = apps.filter((a) => a.failed).length;
  const runningDeps = deps.filter((d) => d.status === "running").length;

  useEffect(() => {
    document.title = `rkt · ${title[0].toUpperCase() + title.slice(1)}`;
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
      <Sidebar collapsible="icon">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton size="lg" onClick={() => navigate("/overview")}>
                <Avatar className="h-8 w-8 rounded-lg text-sm">
                  <AvatarFallback className="rounded-lg bg-signal/15 font-semibold text-signal">
                    {(brand.trim()[0] ?? "R").toUpperCase()}
                  </AvatarFallback>
                </Avatar>
                <div className="grid flex-1 text-left text-sm leading-tight group-data-[collapsible=icon]:hidden">
                  <span className="truncate font-semibold">{brand}</span>
                  <span className="tnum truncate font-mono text-[11px] text-muted-foreground">
                    {apps.length === 0 ? "terminal vacía" : `${running}/${apps.length} en ruta${failed > 0 ? ` · ${failed} varado${failed > 1 ? "s" : ""}` : ""}`}
                  </span>
                </div>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
          {canWrite && (
            <Button className="mt-1 w-full" size="sm" onClick={() => setCreateOpen(true)}>
              <Plus /> <span className="group-data-[collapsible=icon]:hidden">Create New</span>
            </Button>
          )}
          <button
            onClick={() => setPalette(true)}
            className="mt-2 flex w-full items-center justify-between gap-2 rounded-md border border-input bg-transparent px-2 py-1.5 text-xs text-muted-foreground hover:border-ring hover:text-foreground"
          >
            <span className="flex items-center gap-2 group-data-[collapsible=icon]:hidden">
              <Search className="h-3.5 w-3.5" /> Buscar o accionar…
            </span>
            <Search className="hidden h-3.5 w-3.5 group-data-[collapsible=icon]:block" />
            <kbd className="rounded border px-1 font-mono text-[10px] group-data-[collapsible=icon]:hidden">⌘K</kbd>
          </button>
        </SidebarHeader>
        <SidebarContent>
          {GROUPS.map((g) => {
            const items = g.items.filter((n) => !("admin" in n && n.admin) || isAdmin);
            if (items.length === 0) return null;
            return (
              <SidebarGroup key={g.label} className="py-1">
                <SidebarGroupLabel>{g.label}</SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {items.map((n) => (
                      <SidebarMenuItem key={n.to}>
                        <NavLink to={n.to} className="w-full">
                          {({ isActive }) => (
                            <SidebarMenuButton isActive={isActive} tooltip={n.label}>
                              <n.icon />
                              <span>{n.label}</span>
                              {n.to === "/deployments" && runningDeps > 0 && (
                                <Badge variant="secondary" className="tnum ml-auto font-mono text-[10px]">
                                  {runningDeps}
                                </Badge>
                              )}
                              {n.to === "/alerts" && failed > 0 && (
                                <Badge variant="destructive" className="tnum ml-auto font-mono text-[10px]">
                                  {failed}
                                </Badge>
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
          <SidebarSeparator className="mx-0" />
          <SidebarGroup>
            <SidebarGroupLabel className="flex items-center justify-between">
              <span>Terminal</span>
              <span className="tnum font-mono text-[10px]">{apps.length}</span>
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    isActive={appFilter === ""}
                    onClick={() => setAppFilter("")}
                    className="font-mono text-xs"
                  >
                    <span>todas</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                {apps.map((a) => {
                  const st = stateOf(a);
                  const domain = metas.find((m) => m.name === a.name)?.domains[0];
                  return (
                    <SidebarMenuItem key={a.name}>
                      <SidebarMenuButton
                        isActive={appFilter === a.name}
                        onClick={() => openApp(a.name)}
                        className="font-mono text-xs"
                        tooltip={domain ? `${a.name} → ${domain}` : a.name}
                      >
                        <Dot status={st} />
                        <span className="min-w-0 flex-1 truncate">{a.name}</span>
                        {st === "failed" && (
                          <span className="tnum text-[10px] text-red-300">!{a.restarts}</span>
                        )}
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  );
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter>
          {!online && (
            <p className="rounded-md bg-amber-500/15 px-2.5 py-1.5 text-xs text-amber-300 group-data-[collapsible=icon]:hidden">
              Sin conexión — lectura local
            </p>
          )}
          <div className="group-data-[collapsible=icon]:hidden">
            <InstallButton />
          </div>
          <SidebarMenu>
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger className="flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left outline-none hover:bg-accent">
                  <Avatar className="h-6 w-6 text-[11px]">
                    <AvatarFallback>{(who?.user.trim()[0] ?? "?").toUpperCase()}</AvatarFallback>
                  </Avatar>
                  <div className="grid min-w-0 flex-1 text-left text-xs leading-tight group-data-[collapsible=icon]:hidden">
                    <span className="truncate font-medium">{who?.user}</span>
                    <span className="tnum truncate font-mono text-[10px] text-muted-foreground">
                      {who?.role} · v{status?.version ?? "…"}
                    </span>
                  </div>
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
      <SidebarInset>
        <header className="flex items-center gap-2 border-b px-4 py-2.5">
          <SidebarTrigger />
          <nav aria-label="ruta" className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
            <span className="shrink-0 font-medium text-foreground">{brand}</span>
            <span aria-hidden>/</span>
            <span className="truncate capitalize text-foreground">{title}</span>
            {crumbs[1] && (
              <>
                <span aria-hidden>/</span>
                <span className="tnum truncate font-mono">{decodeURIComponent(crumbs[1])}</span>
              </>
            )}
          </nav>
          <span className="flex-1" />
          {!online && <span className="text-xs text-amber-300">offline</span>}
          {failed > 0 ? (
            <Badge variant="destructive" className="tnum font-mono text-[11px] font-normal">
              {failed} varado{failed > 1 ? "s" : ""}
            </Badge>
          ) : (
            <Badge variant="secondary" className="tnum font-mono text-[11px] font-normal">
              {running}/{apps.length} en ruta
            </Badge>
          )}
          {canWrite && (
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus /> Create New
            </Button>
          )}
          <Avatar className="h-6 w-6 text-[11px]">
            <AvatarFallback>{(who?.user.trim()[0] ?? "?").toUpperCase()}</AvatarFallback>
          </Avatar>
        </header>
        <main className="min-w-0 flex-1 p-4 md:p-6">
          <div className="mx-auto max-w-6xl">
            <Err msg={err} />
            {loading ? (
              <div className="space-y-2" aria-label="cargando flota">
                {Array.from({ length: 6 }).map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
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
