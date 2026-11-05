import { useEffect } from "react";
import { NavLink, Link, Outlet, useLocation, useNavigate } from "react-router";
import { toast } from "sonner";
import { Toaster } from "@/components/ui/sonner";
import { LayoutGrid, Plus, Search, ChevronRight, Command } from "lucide-react";
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
import { useI18n } from "../shared/i18n";
import { GROUPS, PAGE_LABEL } from "./nav";

export function ShellLayout() {
  const { t } = useI18n();
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
  const title = t(PAGE_LABEL[crumbs[0] ?? "overview"] ?? "nav.overview");
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
      const t = e.target;
      const typing =
        t instanceof HTMLElement &&
        (t.isContentEditable || t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT");
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPalette(!useUI.getState().palette);
      } else if (e.key === "/" && !typing) {
        e.preventDefault();
        setPalette(true);
      } else if (e.key.toLowerCase() === "n" && !typing && !e.metaKey && !e.ctrlKey && !e.altKey && canWrite) {
        e.preventDefault();
        setCreateOpen(true);
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [setPalette, setCreateOpen, canWrite]);

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
                    <span className="micro">{t("shell.terminal")}</span>
                  </span>
                  <span className="tnum truncate font-mono text-[10px] text-muted-foreground">
                    {apps.length === 0
                      ? t("shell.empty")
                      : failed > 0
                        ? t(failed > 1 ? "shell.runningFailedPlural" : "shell.runningFailed", { running, total: apps.length, failed })
                        : t("shell.running", { running, total: apps.length })}
                  </span>
                </div>
                <span className="micro tnum hidden shrink-0 text-muted-foreground/60 group-data-[collapsible=icon]:hidden xl:inline">
                  v{status?.version ?? "…"}
                </span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>

          {canWrite && (
            <Button className="mt-3 w-full justify-between px-2.5 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0" size="sm" onClick={() => setCreateOpen(true)} aria-keyshortcuts="N">
              <span className="inline-flex items-center gap-1.5">
                <Plus className="h-3.5 w-3.5" />
                <span className="group-data-[collapsible=icon]:hidden">{t("shell.newApp")}</span>
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
            <span className="flex-1 truncate group-data-[collapsible=icon]:hidden">{t("shell.search")}</span>
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
              <SidebarGroup key={g.labelKey} className="py-0 group-data-[collapsible=icon]:p-0">
                <SidebarGroupLabel className="mt-3 mb-1 flex h-4 items-center gap-2 px-2 font-mono text-[9px] tracking-[0.18em] uppercase text-sidebar-foreground/40 group-data-[collapsible=icon]:hidden">
                  <span>{t(g.labelKey)}</span>
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
                              tooltip={t(n.labelKey)}
                              className="relative h-8 rounded-[1px] px-2 text-[12px] font-normal text-sidebar-foreground/65 hover:bg-sidebar-accent hover:text-sidebar-foreground data-active:bg-sidebar-accent data-active:font-medium data-active:text-sidebar-foreground before:absolute before:top-1.5 before:bottom-1.5 before:left-0 before:w-px before:bg-signal before:opacity-0 data-active:before:opacity-100 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0 [&>svg]:size-3.5 [&>svg]:text-muted-foreground/70 data-active:[&>svg]:text-signal"
                            >
                              <n.icon />
                              <span className="flex-1 group-data-[collapsible=icon]:hidden">{t(n.labelKey)}</span>
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

          <SidebarGroup className="mt-3 py-0 group-data-[collapsible=icon]:p-0">
            <SidebarGroupLabel className="mb-1 flex h-4 items-center gap-2 px-2 font-mono text-[9px] tracking-[0.18em] uppercase text-sidebar-foreground/40 group-data-[collapsible=icon]:hidden">
              <span>{t("shell.ports")}</span>
              <span aria-hidden className="h-px flex-1 bg-sidebar-border" />
              <span className="tnum text-[9px] text-sidebar-foreground/55">{apps.length}</span>
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu className="gap-0.5">
                <SidebarMenuItem>
                  <SidebarMenuButton
                    isActive={appFilter === ""}
                    onClick={() => setAppFilter("")}
                    tooltip={t("shell.allPortsTooltip")}
                    className="h-7 rounded-[1px] px-2 font-mono text-[11px] text-sidebar-foreground/55 hover:bg-sidebar-accent hover:text-sidebar-foreground data-active:bg-sidebar-accent data-active:text-sidebar-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
                  >
                    <span className="hidden h-3.5 w-3.5 shrink-0 items-center justify-center text-muted-foreground/60 group-data-[collapsible=icon]:flex">
                      <LayoutGrid className="h-3.5 w-3.5" />
                    </span>
                    <span className="flex-1 group-data-[collapsible=icon]:hidden">{t("shell.allPorts")}</span>
                    <span className="tnum text-[10px] text-muted-foreground/60 group-data-[collapsible=icon]:hidden">{apps.length}</span>
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
                        className="h-7 rounded-[1px] px-2 font-mono text-[11px] text-sidebar-foreground/60 hover:bg-sidebar-accent hover:text-sidebar-foreground data-active:bg-sidebar-accent data-active:text-sidebar-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
                        tooltip={domain ? `${a.name} → ${domain}` : a.name}
                      >
                        <Dot status={st} />
                        <span className="min-w-0 flex-1 truncate group-data-[collapsible=icon]:hidden">{a.name}</span>
                        {meta?.port && (
                          <span className="tnum text-[10px] text-muted-foreground/55 group-data-[collapsible=icon]:hidden">:{meta.port}</span>
                        )}
                        {st === "failed" && (
                          <span className="tnum text-[10px] text-bad group-data-[collapsible=icon]:hidden">!{a.restarts}</span>
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
              {t("shell.noConnection")}
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
                    {t("shell.logout")}
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
          <nav aria-label={t("shell.crumb")} className="flex min-w-0 items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
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
            {new Date().toLocaleDateString("en-CA")}
          </span>
          {!online && <span className="font-mono text-[10px] text-warn">{t("shell.offline")}</span>}
          <Link
            to="/alerts"
            aria-label={failed > 0
              ? t("shell.alertsAriaFailed", { n: failed })
              : t("shell.alertsAriaOk", { running, total: apps.length })}
            className="hidden items-center gap-1.5 border border-border bg-card/40 px-2 py-1 transition-colors hover:border-input hover:bg-card/70 md:flex"
          >
            <Dot status={failed > 0 ? "failed" : "ok"} />
            <span className="tnum font-mono text-[10px] text-muted-foreground">
              {failed > 0
                ? t(failed > 1 ? "shell.failedMany" : "shell.failedOne", { n: failed })
                : t("shell.running", { running, total: apps.length })}
            </span>
          </Link>
          {canWrite && (
            <Button size="sm" onClick={() => setCreateOpen(true)} className="h-7 md:hidden">
              <Plus className="h-3.5 w-3.5" /> {t("shell.newApp")}
            </Button>
          )}
          <Avatar name={who?.user ?? "?"} className="h-6 w-6" />
        </header>
        <main className="min-w-0 flex-1 px-3 py-5 md:px-5 md:py-6">
          <div className="mx-auto max-w-[1480px]">
            <Err msg={err} />
            {loading ? (
              <div className="space-y-px border border-border" aria-busy="true" aria-label={t("shell.loadingFleet")}>
                <span className="sr-only">{t("shell.loadingFleet")}</span>
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
  const { t } = useI18n();
  const createOpen = useUI((u) => u.createOpen);
  const setCreateOpen = useUI((u) => u.setCreateOpen);
  const fail = useUI((u) => u.fail);
  return (
    <CreateDialog
      open={createOpen}
      setOpen={setCreateOpen}
      onCreated={(name) => {
        toast.success(t("create.created", { name }));
      }}
      onOpen={(name) => openApp(name)}
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
