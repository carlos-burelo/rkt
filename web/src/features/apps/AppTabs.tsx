import { useEffect } from "react";
import { Outlet, useLocation, useNavigate, useParams } from "react-router";
import { ExternalLink, RotateCw } from "lucide-react";
import { toast } from "sonner";
import { Dot, StatusBadge } from "../../shared/components/ui";
import { stateLabel } from "../../shared/lib/labels";
import { Button, buttonVariants } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { fixHost } from "../routes/api";

const TABS = [
  { id: "overview", label: "Resumen" },
  { id: "deploys", label: "Despliegues" },
  { id: "logs", label: "Registros" },
  { id: "analytics", label: "Analítica" },
  { id: "env", label: "Entorno" },
  { id: "settings", label: "Ajustes" },
] as const;

function Cell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0 bg-card/45 px-3 py-2">
      <p className="micro">{label}</p>
      <div className="mt-1 truncate font-mono text-[11px]">{children}</div>
    </div>
  );
}

// AppTabs ficha por app con tabs anidadas (el scroll-muro murió).
// Fija appFilter para que Deployments/Logs reutilicen su filtro.
export function AppTabs() {
  const { name = "" } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const { status, metas } = useFleet();
  const act = useUI((u) => u.act);
  const setAppFilter = useUI((u) => u.setAppFilter);
  const { canWrite } = useAuth();
  const active = location.pathname.split("/").pop() ?? "overview";

  useEffect(() => {
    if (name) setAppFilter(name);
  }, [name, setAppFilter]);

  const meta = metas.find((m) => m.name === name);
  const st = status?.apps.find((a) => a.name === name);
  const state = st?.failed ? "failed" : st?.running ? (st.http_ok === false ? "degraded" : "running") : "stopped";
  const domains = meta?.domains ?? [];

  return (
    <div className="space-y-4">
      <header className="border border-border bg-card/35">
        <div className="flex flex-wrap items-start gap-3 border-b border-border px-4 py-4">
          <span aria-hidden className="mt-1.5 h-5 w-1 shrink-0 bg-signal" />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate font-mono text-[24px] leading-none font-medium tracking-[-0.04em]">{name || "(sin app)"}</h1>
              <Dot status={state} />
              <StatusBadge status={state} />
            </div>
            <p className="mt-2 font-mono text-[10px] text-muted-foreground">
              unidad de flota · puerto {meta?.port ? `:${meta.port}` : "sin asignar"} ·{" "}
              {domains.length > 0 ? `${domains.length} destino${domains.length > 1 ? "s" : ""}` : "sin destino"}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {domains[0] && (
              <a
                href={`https://${domains[0]}`}
                target="_blank"
                rel="noreferrer"
                className={buttonVariants({ variant: "outline", size: "sm" })}
              >
                <ExternalLink className="h-3 w-3" /> Abrir
              </a>
            )}
            {canWrite && name && (
              <>
                <Button variant="outline" size="sm" onClick={() => act(name, "restart")}>
                  <RotateCw className="h-3 w-3" /> Reiniciar
                </Button>
                <Button size="sm" onClick={() => act(name, "deploy")}>Redesplegar</Button>
                <Button variant="destructive" size="sm" onClick={() => act(name, "rollback")}>Revertir</Button>
              </>
            )}
          </div>
        </div>
        <div className="grid grid-cols-2 gap-px bg-border md:grid-cols-4">
          <Cell label="estado">
            <span className={st?.failed ? "text-bad" : st?.running ? "text-ok" : "text-muted-foreground"}>
              {stateLabel(state)}
            </span>
          </Cell>
          <Cell label="reinicios">
            <span className="tnum">{st?.restarts ?? 0}</span>
          </Cell>
          <Cell label="destino">
            {domains[0] ? (
              <span className="flex min-w-0 items-center gap-1.5">
                <a href={`https://${domains[0]}`} target="_blank" rel="noreferrer" className="truncate hover:text-signal hover:underline">
                  {domains[0]}
                </a>
                {meta?.dns_ok === false && (
                  <span className="flex shrink-0 items-center gap-1.5">
                    <span className="font-mono text-[9px] text-warn" title="el dominio no resuelve a 127.0.0.1 en esta máquina">
                      no resuelve
                    </span>
                    {canWrite && (
                      <Button
                        variant="outline"
                        size="xs"
                        onClick={() => {
                          fixHost(domains[0]).then((r) => {
                            if (r.ok) toast.success(`${domains[0]} añadido a hosts`);
                            else toast.error(r.hint ?? r.error ?? "no se pudo escribir hosts");
                          });
                        }}
                      >
                        arreglar
                      </Button>
                    )}
                  </span>
                )}
              </span>
            ) : (
              <span className="text-muted-foreground">sin dominio</span>
            )}
          </Cell>
          <Cell label="comando">
            <span className="text-muted-foreground">
              {meta?.static ? `estático · ${meta.static}` : meta?.command || "—"}
            </span>
          </Cell>
        </div>
      </header>
      <Tabs
        value={TABS.some((t) => t.id === active) ? active : "overview"}
        onValueChange={(v) => {
          if (v) navigate(v);
        }}
      >
        {/* scroll horizontal: 6 tabs no caben a 390px sin desbordar */}
        <div className="-mx-1 overflow-x-auto px-1">
          <TabsList className="w-max gap-5 sm:w-full">
            {TABS.map((t) => (
              <TabsTrigger key={t.id} value={t.id}>
                {t.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
      </Tabs>
      <Outlet />
    </div>
  );
}
