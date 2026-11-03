import { useNavigate } from "react-router";
import { ArrowRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Avatar, Dot, stateOf } from "../../shared/components/ui";
import { CopyButton } from "../../shared/components/feedback";
import { Metrics } from "./Metrics";
import { UsageStrip } from "./UsageStrip";
import { useFleet } from "./hooks";
import { useAuth } from "../auth/store";

// Overview: manifiesto de la terminal. Cada app es un contenedor:
// glyph + nombre + destino (dominio) + rev en curso + estado.
// Sin tarjetas-métrica genéricas: las placas llevan ventana honesta.
export function Overview() {
  const { status, evs, deps, metas, loading } = useFleet();
  const { canWrite } = useAuth();
  const navigate = useNavigate();
  const openApp = (name: string) => navigate(`/app/${name}`);
  const apps = status?.apps ?? [];

  if (loading) return <p className="text-sm text-muted-foreground">cargando…</p>;

  const discovered = evs.filter((e) => e.kind === "discover").slice(0, 3);
  const openAlerts = evs.filter((e) => e.kind === "alert").slice(0, 3);
  const recent = deps.slice(0, 5);

  return (
    <div className="space-y-4">
      <Metrics apps={apps} deps={deps} />

      {openAlerts.length > 0 && (
        <section aria-label="alertas abiertas" className="overflow-hidden rounded-lg border border-amber-500/30">
          {openAlerts.map((e) => (
            <div key={e.id} className="flex items-center gap-3 border-b border-amber-500/20 bg-amber-500/10 px-4 py-3 last:border-0">
              <Dot status="degraded" />
              <div className="min-w-0 flex-1">
                <p className="tnum truncate font-mono text-xs">{e.app} · {e.msg}</p>
              </div>
              <Button variant="outline" size="sm" onClick={() => navigate("/alerts")}>
                ver alerta
              </Button>
            </div>
          ))}
        </section>
      )}

      {discovered.length > 0 && (
        <section aria-label="repos detectados" className="overflow-hidden rounded-lg border">
          {discovered.map((e) => (
            <div key={e.id} className="flex items-center gap-3 border-b bg-card px-4 py-3 last:border-0">
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">New repository detected</p>
                <p className="tnum truncate font-mono text-xs text-muted-foreground">{e.msg}</p>
              </div>
              {canWrite ? (
                <Button size="sm" onClick={() => openApp(e.app)}>
                  Import
                </Button>
              ) : (
                <Badge variant="outline" className="font-mono text-[11px]">{e.app}</Badge>
              )}
            </div>
          ))}
        </section>
      )}

      <section aria-label="apps">
        <div className="mb-2 flex items-baseline justify-between">
          <h3 className="text-sm font-medium">Contenedores · {apps.length}</h3>
          <p className="font-mono text-[11px] text-muted-foreground">nombre · destino · estado</p>
        </div>
        <div className="overflow-hidden rounded-lg border">
          {apps.map((a) => {
            const st = stateOf(a);
            const meta = metas.find((m) => m.name === a.name);
            const domain = meta?.domains[0];
            const last = deps.find((d) => d.app === a.name);
            return (
              <button
                key={a.name}
                onClick={() => openApp(a.name)}
                className="group flex w-full items-center gap-3 border-b bg-card/40 px-4 py-3 text-left last:border-0 hover:bg-accent/40"
              >
                <span aria-hidden className="manifest-strip h-8 w-1 shrink-0 rounded-full opacity-70" />
                <Avatar name={a.name} />
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-sm">{a.name}</p>
                  <p className="tnum truncate font-mono text-[11px] text-muted-foreground">
                    {domain ?? "sin dominio"}{last ? ` · ${last.rev.slice(0, 8)}` : ""}
                  </p>
                </div>
                <span className="tnum hidden font-mono text-[11px] text-muted-foreground sm:block">
                  {a.restarts} restarts
                </span>
                <Badge
                  variant={st === "failed" ? "destructive" : st === "running" ? "secondary" : "outline"}
                  className="tnum font-mono text-[11px] font-normal"
                >
                  <Dot status={st} />
                  <span className="ml-1.5">{st}</span>
                </Badge>
                <span aria-hidden className="text-muted-foreground transition-transform group-hover:translate-x-0.5"><ArrowRight className="h-3.5 w-3.5" /></span>
              </button>
            );
          })}
          {apps.length === 0 && (
            <div className="p-6">
              <p className="text-sm font-medium">Terminal vacía</p>
              <p className="mt-1 font-mono text-xs text-muted-foreground">
                Registra tu primera app con <span className="text-foreground">rkt apps add --name mi-app --port 3000</span>
              </p>
            </div>
          )}
        </div>
      </section>

      {recent.length > 0 && (
        <section aria-label="salidas recientes">
          <div className="mb-2 flex items-baseline justify-between">
            <h3 className="text-sm font-medium">Últimas salidas</h3>
            <Button variant="ghost" size="sm" className="h-6 text-xs" onClick={() => navigate("/deployments")}>
              tablero completo <ArrowRight className="h-3.5 w-3.5" />
            </Button>
          </div>
          <div className="overflow-hidden rounded-lg border">
            {recent.map((d) => (
              <div
                key={d.id}
                className="flex items-center gap-3 border-b bg-card/40 px-4 py-2.5 last:border-0"
              >
                <Dot status={d.status} />
                <span className="truncate font-mono text-xs">{d.app}</span>
                <span className="tnum font-mono text-[11px] text-muted-foreground">#{d.id} · {d.rev.slice(0, 8)}</span>
                <CopyButton text={d.rev} />
                <span className="flex-1" />
                <span className="tnum font-mono text-[11px] text-muted-foreground">
                  {d.status === "running" ? "en curso…" : d.triggered_by ? `por ${d.triggered_by}` : ""}
                </span>
              </div>
            ))}
          </div>
        </section>
      )}

      <UsageStrip />
    </div>
  );
}
