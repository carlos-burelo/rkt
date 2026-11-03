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
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent,
  SidebarGroupLabel, SidebarHeader, SidebarInset, SidebarMenu,
  SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarRail, SidebarTrigger,
} from "@/components/ui/sidebar";
import { Dot, Err } from "../shared/components/ui";
import { InstallButton } from "../shared/components/InstallButton";
import { CommandPalette } from "../shared/components/CommandPalette";
import { useAuth } from "../features/auth/store";
import { useUI } from "./ui";
import { useFleet } from "../features/overview/hooks";
import { CreateDialog } from "../features/apps/CreateDialog";
import { DeployDrawer } from "../features/deployments/DeployDrawer";

const NAV = [
  { to: "/overview", label: "Overview", icon: LayoutGrid },
  { to: "/proxy", label: "Proxy", icon: Network },
  { to: "/deployments", label: "Deployments", icon: Rocket },
  { to: "/ci", label: "CI", icon: Hammer },
  { to: "/logs", label: "Logs", icon: Terminal },
  { to: "/analytics", label: "Analytics", icon: BarChart3 },
  { to: "/observability", label: "Observability", icon: Activity },
  { to: "/alerts", label: "Alerts", icon: Bell },
  { to: "/routes", label: "Domains", icon: Globe },
  { to: "/environment", label: "Environment", icon: FlaskConical },
  { to: "/settings", label: "Settings", icon: SettingsIcon },
  { to: "/users", label: "Users", icon: UsersIcon, admin: true },
] as const;

// ShellLayout armazón (sidebar/topbar/drawers) + <Outlet/> por ruta.
// Datos: cada ruta carga lo suyo (useFleet); aquí solo chrome + UI.
export function ShellLayout() {
  const { who, isAdmin, canWrite, brand, doLogout } = useAuth();
  const online = useUI((u) => u.online);
  const setOnline = useUI((u) => u.setOnline);
  const palette = useUI((u) => u.palette);
  const setPalette = useUI((u) => u.setPalette);
  const createOpen = useUI((u) => u.setCreateOpen);
  const setCreateOpen = useUI((u) => u.setCreateOpen);
  const appFilter = useUI((u) => u.appFilter);
  const setAppFilter = useUI((u) => u.setAppFilter);
  const err = useUI((u) => u.err);
  const { status, deps, metas, loading } = useFleet();
  const location = useLocation();
  const navigate = useNavigate();
  const title = location.pathname.split("/")[1] || "overview";

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
                  <AvatarFallback className="rounded-lg">{(brand.trim()[0] ?? "R").toUpperCase()}</AvatarFallback>
                </Avatar>
                <div className="grid flex-1 text-left text-sm leading-tight group-data-[collapsible=icon]:hidden">
                  <span className="truncate font-semibold">{brand}</span>
                  <span className="truncate text-xs text-muted-foreground">local</span>
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
            className="mt-2 flex w-full items-center justify-between rounded-md border border-input bg-transparent px-2 py-1.5 text-xs text-muted-foreground hover:border-ring"
          >
            <span className="flex items-center gap-2 group-data-[collapsible=icon]:hidden">
              <Search className="h-3.5 w-3.5" /> Buscar o accionar…
            </span>
            <Search className="hidden h-3.5 w-3.5 group-data-[collapsible=icon]:block" />
            <kbd className="rounded border px-1 font-mono text-[10px] group-data-[collapsible=icon]:hidden">⌘K</kbd>
          </button>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                {NAV.filter((n) => !("admin" in n && n.admin) || isAdmin).map((n) => (
                  <SidebarMenuItem key={n.to}>
                    <NavLink to={n.to} className="w-full">
                      {({ isActive }) => (
                        <SidebarMenuButton isActive={isActive} tooltip={n.label}>
                          <n.icon />
                          <span>{n.label}</span>
                        </SidebarMenuButton>
                      )}
                    </NavLink>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
          <SidebarGroup>
            <SidebarGroupLabel>Apps</SidebarGroupLabel>
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
                {(status?.apps ?? []).map((a) => (
                  <SidebarMenuItem key={a.name}>
                    <SidebarMenuButton
                      isActive={appFilter === a.name}
                      onClick={() => openApp(a.name)}
                      className="font-mono text-xs"
                      tooltip={a.name}
                    >
                      <Dot status={a.failed ? "failed" : a.running ? "running" : "stopped"} />
                      <span>{a.name}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter>
          {!online && (
            <p className="rounded-md bg-amber-500/15 px-2.5 py-1.5 text-xs text-amber-300 group-data-[collapsible=icon]:hidden">
              Sin conexión
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
                    <span className="truncate font-mono text-[10px] text-muted-foreground">
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
          <span className="text-xs text-muted-foreground">▲ {brand}</span>
          <span className="flex-1 text-center text-sm font-medium capitalize">{title}</span>
          {!online && <span className="text-xs text-amber-300">offline</span>}
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
          <div className="mx-auto max-w-5xl">
            <Err msg={err} />
            {loading ? (
              <div className="space-y-2">
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
