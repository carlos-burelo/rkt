import { useEffect } from "react";
import { Outlet, useNavigate, useParams } from "react-router";
import { ExternalLink, Play, RotateCw, Square } from "lucide-react";
import { toast } from "sonner";
import { Dot, StatusBadge } from "../../shared/components/ui";
import { stateLabel } from "../../shared/lib/labels";
import { Button, buttonVariants } from "@/components/ui/button";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { RowMenu } from "../../shared/components/menus";
import { useFleet } from "../overview/hooks";
import { publicUrl, publicHost } from "../../shared/lib/url";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { fixHost } from "../routes/api";
import { useI18n } from "../../shared/i18n";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { buildApp } from "./api";

function Cell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0 bg-card/45 px-3 py-2">
      <p className="micro">{label}</p>
      <div className="mt-1 truncate font-mono text-[12px]">{children}</div>
    </div>
  );
}

// AppTabs ficha por app con tabs anidadas (el scroll-muro murió).
// Fija appFilter para que Deployments/Logs reutilicen su filtro.
export function AppTabs() {
  const { t, tn } = useI18n();
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const { status, metas } = useFleet();
  const act = useUI((u) => u.act);
  const setAppFilter = useUI((u) => u.setAppFilter);
  const { canWrite } = useAuth();

  useEffect(() => {
    if (name) setAppFilter(name);
  }, [name, setAppFilter]);

  const meta = metas.find((m) => m.name === name);
  const st = status?.apps.find((a) => a.name === name);
  const state = meta?.disabled ? "stopped" : st?.failed ? "failed" : st?.running ? (st.http_ok === false ? "degraded" : "running") : "stopped";
  const domains = meta?.domains ?? [];
  const external = !!meta?.external;
  const port = meta?.port ? `:${meta.port}` : t("apps.header.portUnassigned");
  const destinations =
    domains.length > 0
      ? tn("apps.header.destinations", domains.length)
      : t("apps.header.noDestination");

  return (
    <div className="space-y-4">
      <header className="border border-border bg-card/35">
        <div className="flex flex-wrap items-start gap-3 border-b border-border px-4 py-4">
          <span aria-hidden className="mt-1.5 h-5 w-1 shrink-0 bg-signal" />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate font-mono text-[24px] leading-none font-medium tracking-[-0.04em]">{name || t("apps.tabs.noApp")}</h1>
              {external && (
                <span className="border border-info/40 bg-info/10 px-1.5 font-mono text-[9px] tracking-[0.08em] text-info uppercase">
                  {t("apps.tabs.external")}
                </span>
              )}
              <Dot status={state} />
              <StatusBadge status={state} />
            </div>
            <p className="mt-2 font-mono text-[11px] text-muted-foreground">
              {t("apps.header.meta", { port, destinations })}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {domains[0] && (
              <a
                href={publicUrl(domains[0], status)}
                target="_blank"
                rel="noreferrer"
                className={buttonVariants({ variant: "outline", size: "sm" })}
              >
                <ExternalLink className="h-3 w-3" /> {t("common.open")}
              </a>
            )}
            {canWrite && name && !external && (
              <>
                {meta?.deploy?.repo ? (
                  <Button size="sm" onClick={() => act(name, "deploy")}>{t("apps.actions.redeploy")}</Button>
                ) : (
                  <Button
                    size="sm"
                    onClick={() =>
                      buildApp(name)
                        .then(() => {
                          toast.success(t("apps.toast.buildStarted"));
                          navigate(`/app/${name}/deploys`);
                        })
                        .catch((e) => toast.error(e instanceof Error ? e.message : t("common.error")))
                    }
                  >
                    {t("apps.actions.build")}
                  </Button>
                )}
                <RowMenu label={t("apps.actions.menu", { app: name })}>
                  {meta?.disabled ? (
                    <DropdownMenuItem onClick={() => act(name, "start")}>
                      <Play className="h-3 w-3" /> {t("apps.actions.start")}
                    </DropdownMenuItem>
                  ) : (
                    <DropdownMenuItem onClick={() => act(name, "stop")}>
                      <Square className="h-3 w-3" /> {t("apps.actions.stop")}
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuItem onClick={() => act(name, "restart")}>
                    <RotateCw className="h-3 w-3" /> {t("apps.actions.restart")}
                  </DropdownMenuItem>
                  <ConfirmButton
                    label={t("apps.actions.rollback")}
                    confirmLabel={t("apps.actions.rollbackConfirm")}
                    onConfirm={() => act(name, "rollback")}
                    size="sm"
                  />
                </RowMenu>
              </>
            )}
          </div>
        </div>
        <div className="grid grid-cols-2 gap-px bg-border lg:grid-cols-4">
          <Cell label={t("apps.field.state")}>
            <span className={st?.failed ? "text-bad" : st?.running ? "text-ok" : "text-muted-foreground"}>
              {stateLabel(state)}
            </span>
          </Cell>
          <Cell label={t("apps.field.restarts")}>
            <span className="tnum">{st?.restarts ?? 0}</span>
          </Cell>
          <Cell label={t("apps.field.destination")}>
            {domains[0] ? (
              <span className="flex min-w-0 items-center gap-1.5">
                <a href={publicUrl(domains[0], status)} target="_blank" rel="noreferrer" className="truncate hover:text-signal hover:underline">
                  {publicHost(domains[0], status)}
                </a>
                {meta?.dns_ok === false && (
                  <span className="flex shrink-0 items-center gap-1.5">
                    <span className="font-mono text-[9px] text-warn" title={t("apps.tabs.dnsTitle")}>
                      {t("apps.tabs.dnsUnresolved")}
                    </span>
                    {canWrite && (
                      <Button
                        variant="outline"
                        size="xs"
                        onClick={() => {
                          fixHost(domains[0]).then((r) => {
                            if (r.ok) toast.success(t("apps.toast.hostAdded", { domain: domains[0] }));
                            else toast.error(r.hint ?? r.error ?? t("apps.toast.hostError"));
                          });
                        }}
                      >
                        {t("apps.tabs.fixHost")}
                      </Button>
                    )}
                  </span>
                )}
              </span>
            ) : (
              <span className="text-muted-foreground">{t("apps.tabs.noDomain")}</span>
            )}
          </Cell>
          <Cell label={t("apps.field.command")}>
            <span className="text-muted-foreground">
              {meta?.static ? t("apps.tabs.staticValue", { path: meta.static }) : meta?.command || "—"}
            </span>
          </Cell>
        </div>
      </header>
      <Outlet />
    </div>
  );
}
