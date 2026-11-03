import { useEffect, useRef, useState, type ComponentType } from "react";
import { toast } from "sonner";
import { Toaster } from "@/components/ui/sonner";
import {
  LayoutGrid, Network, Rocket, Hammer, Terminal, BarChart3, Activity,
  Bell, Globe, FlaskConical, Settings as SettingsIcon, Users as UsersIcon,
  Plus, ChevronUp, Search,
} from "lucide-react";
import { api } from "../shared/lib/http";
import { Me, Status, Deployment, RktEvent, AppMeta, User } from "../shared/lib/types";
import { logout, me, authStatus } from "../features/auth/api";
import { Login } from "../features/auth/Login";
import { Setup } from "../features/auth/Setup";
import { Dot, Err } from "../shared/components/ui";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent,
  SidebarGroupLabel, SidebarHeader, SidebarInput, SidebarInset, SidebarMenu,
  SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarRail, SidebarTrigger,
} from "@/components/ui/sidebar";
import { InstallButton } from "../shared/components/InstallButton";
import { CommandPalette } from "../shared/components/CommandPalette";
import { useOnline } from "../shared/hooks/useOnline";
import { DeployDrawer } from "../features/deployments/DeployDrawer";
import { Overview } from "../features/overview/Overview";
import { Proxy } from "../features/proxy/Proxy";
import { CreateDialog } from "../features/apps/CreateDialog";
import { AnalyticsPage } from "../features/overview/AnalyticsPage";
import { Observability } from "../features/overview/Observability";
import { Alerts } from "../features/alerts/Alerts";
import { EnvAll } from "../features/env/EnvAll";
import { CI } from "../features/ci/CI";
import { Deployments } from "../features/deployments/Deployments";
import { AppDetail } from "../features/apps/AppDetail";
import { Logs } from "../features/logs/Logs";
import { Routes } from "../features/routes/Routes";
import { Settings } from "../features/settings/Settings";
import { Users } from "../features/users/Users";
import { fetchStatus, fetchDeployments, fetchEvents } from "../features/overview/api";
import { deploymentLog, retryDeployment } from "../features/deployments/api";
import { listApps, appAction, getEnv } from "../features/apps/api";
import { fetchLog } from "../features/logs/api";
import { listRoutes } from "../features/routes/api";
import { getSettings } from "../features/settings/api";
import { listUsers } from "../features/users/api";

export type Page = "overview" | "deployments" | "logs" | "routes" | "settings" | "users" | "app" | "analytics" | "observability" | "environment" | "ci" | "alerts" | "proxy";

const NAV: { id: Page; label: string; icon: ComponentType<{ className?: string }>; admin?: boolean }[] = [
  { id: "overview", label: "Overview", icon: LayoutGrid },
  { id: "proxy", label: "Proxy", icon: Network },
  { id: "deployments", label: "Deployments", icon: Rocket },
  { id: "ci", label: "CI", icon: Hammer },
  { id: "logs", label: "Logs", icon: Terminal },
  { id: "analytics", label: "Analytics", icon: BarChart3 },
  { id: "observability", label: "Observability", icon: Activity },
  { id: "alerts", label: "Alerts", icon: Bell },
  { id: "routes", label: "Domains", icon: Globe },
  { id: "environment", label: "Environment", icon: FlaskConical },
  { id: "settings", label: "Settings", icon: SettingsIcon },
  { id: "users", label: "Users", icon: UsersIcon, admin: true },
];

export function App() {
  const [who, setWho] = useState<Me | null>(null);
  const online = useOnline();
  const authed = who !== null;
  const isAdmin = who?.role === "admin";
  const canWrite = who?.role === "admin" || who?.role === "operator";
  const [page, setPage] = useState<Page>("deployments");
  const [appFilter, setAppFilter] = useState("");
  const [status, setStatus] = useState<Status | null>(null);
  const [metas, setMetas] = useState<AppMeta[]>([]);
  const [deps, setDeps] = useState<Deployment[]>([]);
  const [evs, setEvs] = useState<RktEvent[]>([]);
  const [log, setLog] = useState("");
  const [follow, setFollow] = useState(true);
  const [err, setErr] = useState("");
  const [boot, setBoot] = useState(true);
  const [sel, setSel] = useState<Deployment | null>(null);
  const [buildLog, setBuildLog] = useState("");
  const [env, setEnvMap] = useState<Record<string, string>>({});
  const [routes, setRoutes] = useState<{ hosts: string[]; target: string }[]>([]);
  const [settings, setSettingsMap] = useState<Record<string, string>>({});
  const [users, setUsers] = useState<User[]>([]);
  const [palette, setPalette] = useState(false);
  const [brand, setBrand] = useState("rkt");
  const [setupRequired, setSetupRequired] = useState<boolean | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const logRef = useRef<HTMLPreElement>(null);

  function fail(e: unknown) {
    if (e instanceof Error && e.message.startsWith("no autorizado")) {
      setWho(null);
      return;
    }
    setErr(e instanceof Error ? e.message : "error");
  }

  async function refresh() {
    try {
      setErr("");
      const [st, dp, ev, am] = await Promise.all([
        fetchStatus(),
        fetchDeployments(),
        fetchEvents(),
        listApps(),
      ]);
      setStatus(st);
      setDeps(dp);
      setEvs(ev);
      setMetas(am);
      if (page === "logs") setLog(await fetchLog(appFilter));
      if (page === "routes") setRoutes(await listRoutes());
      if (page === "settings") setSettingsMap(await getSettings());
      if (page === "users" && isAdmin) setUsers(await listUsers());
      if (page === "app" && appFilter) setEnvMap(await getEnv(appFilter));
    } catch (e) {
      fail(e);
    } finally {
      setBoot(false);
    }
  }

  useEffect(() => {
    authStatus()
      .then((s) => {
        setSetupRequired(s.setup_required);
        if (s.setup_required) return;
        return me()
          .then((m) => {
            setWho(m);
            getSettings()
              .then((gs) => {
                if (gs.site_name) setBrand(gs.site_name);
              })
              .catch(() => {});
          })
          .catch(() => setWho(null));
      })
      .catch(() => {
        setSetupRequired(false);
        setWho(null);
      });
  }, []);

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPalette((v) => !v);
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
  }, []);

  useEffect(() => {
    if (!authed || !online) return;
    refresh();
    const t = setInterval(refresh, 5000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authed, online, page, appFilter]);

  useEffect(() => {
    if (follow && logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [log, follow]);

  useEffect(() => {
    if (sel) {
      setBuildLog("cargando…");
      deploymentLog(sel.id)
        .then(setBuildLog)
        .catch((e) => setBuildLog(e instanceof Error ? e.message : "sin log"));
    }
  }, [sel]);

  if (setupRequired === null) return null;
  if (setupRequired) return <Setup onOk={setWho} />;
  if (!authed) return <Login onOk={setWho} />;

  const apps = status?.apps ?? [];
  const appMeta = metas.find((m) => m.name === appFilter);

  async function act(app: string, a: "restart" | "deploy" | "rollback") {
    try {
      await appAction(app, a);
      toast.success(`${a} ${app} ok`);
      refresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "error");
    }
  }

  function openApp(name: string) {
    setAppFilter(name);
    setPage("app");
  }

  return (
    <SidebarProvider>
      <Toaster theme="dark" position="bottom-right" />
      <Sidebar collapsible="icon">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton size="lg" onClick={() => setPage("overview")}>
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
                {NAV.filter((n) => !n.admin || isAdmin).map((n) => (
                  <SidebarMenuItem key={n.id}>
                    <SidebarMenuButton
                      isActive={page === n.id || (n.id === "deployments" && page === "app")}
                      onClick={() => setPage(n.id)}
                      tooltip={n.label}
                    >
                      <n.icon />
                      <span>{n.label}</span>
                    </SidebarMenuButton>
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
                {apps.map((a) => (
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
                  <DropdownMenuItem onClick={() => logout().finally(() => setWho(null))}>
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
          <span className="text-xs text-muted-foreground">▲ {brand}{page === "app" ? ` / ${appFilter}` : ""}</span>
          <span className="flex-1 text-center text-sm font-medium capitalize">
            {page === "app" ? appFilter || "app" : page}
          </span>
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
            {boot ? (
              <div className="space-y-2">
                {Array.from({ length: 6 }).map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : (
              <>
                {page === "overview" && <Overview apps={apps} evs={evs} deps={deps} metas={metas} openApp={openApp} />}
                {page === "proxy" && <Proxy fail={fail} />}
                {page === "deployments" && (
                  <Deployments deps={deps} appFilter={appFilter} onSelect={setSel} />
                )}
                {page === "app" && (
                  <AppDetail
                    app={appFilter}
                    meta={appMeta}
                    status={apps.find((a) => a.name === appFilter)}
                    env={env}
                    deployments={deps.filter((d) => d.app === appFilter)}
                    canWrite={canWrite}
                    onAction={act}
                    onOpenDeploy={setSel}
                    onDeleted={() => {
                      setAppFilter("");
                      setPage("deployments");
                      refresh();
                    }}
                    refresh={refresh}
                    fail={fail}
                  />
                )}
                {page === "logs" && (
                  <Logs
                    apps={apps}
                    appFilter={appFilter}
                    setAppFilter={setAppFilter}
                    log={log}
                    setLog={setLog}
                    follow={follow}
                    setFollow={setFollow}
                    refresh={refresh}
                    logRef={logRef}
                    deps={deps}
                  />
                )}
                {page === "routes" && <Routes routes={routes} canWrite={canWrite} refresh={refresh} fail={fail} />}
                {page === "settings" && <Settings settings={settings} canWrite={canWrite} refresh={refresh} fail={fail} />}
                {page === "users" && isAdmin && <Users users={users} refresh={refresh} fail={fail} />}
                {page === "analytics" && <AnalyticsPage />}
                {page === "observability" && <Observability />}
                {page === "alerts" && <Alerts canWrite={canWrite} fail={fail} />}
                {page === "environment" && <EnvAll fail={fail} />}
                {page === "ci" && <CI deps={deps} onSelect={setSel} />}
              </>
            )}
          </div>
        </main>
      </SidebarInset>

      <CreateDialog
        open={createOpen}
        setOpen={setCreateOpen}
        onCreated={(name) => {
          toast.success(`app ${name} creada`);
          refresh();
          openApp(name);
        }}
        goDomains={() => setPage("routes")}
        fail={fail}
      />
      <DeployDrawer
        dep={sel}
        buildLog={buildLog}
        canWrite={canWrite}
        onAction={act}
        onRetry={(id) => {
          retryDeployment(id)
            .then((rev) => {
              toast.success(`retry ok ${rev.slice(0, 8)}`);
              setSel(null);
              refresh();
            })
            .catch((e) => toast.error(e instanceof Error ? e.message : "error"));
        }}
        onClose={() => setSel(null)}
      />
      <CommandPalette
        open={palette}
        setOpen={setPalette}
        apps={apps}
        metas={metas}
        deps={deps}
        goPage={(p) => setPage(p)}
        openApp={openApp}
        openDeploy={setSel}
        onAction={act}
      />
    </SidebarProvider>
  );
}
