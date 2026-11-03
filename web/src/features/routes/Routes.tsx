import { useEffect, useRef, useState } from "react";
import { ExternalLink } from "lucide-react";
import { Route } from "../../shared/lib/types";
import { addRoute, delRoute, putRoute, tlsStatus, listRoutes } from "./api";
import { fetchProxy } from "../proxy/api";
import { buildLinks, GraphLink } from "../proxy/graph";
import { FunnelMap } from "./FunnelMap";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { CopyButton } from "../../shared/components/feedback";
import { Avatar } from "../../shared/components/ui";
import { PageHeader, EmptyState, Section } from "../../shared/components/patterns";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

function TLSBadge({ host }: { host: string }) {
  const [txt, setTxt] = useState("…");
  const [ok, setOk] = useState(false);
  useEffect(() => {
    tlsStatus(host)
      .then((t) => {
        setOk(t.managed);
        setTxt(t.expires || "?");
      })
      .catch(() => {
        setOk(false);
        setTxt("?");
      });
  }, [host]);
  return (
    <Badge
      variant={ok ? "outline" : "secondary"}
      className={ok ? "border-emerald-500/30 bg-emerald-500/15 font-mono text-[11px] text-emerald-300" : "font-mono text-[11px]"}
      title={txt}
    >
      {ok ? `TLS ${txt}` : "sin TLS"}
    </Badge>
  );
}

function RouteRow({ r, onChanged }: { r: Route; onChanged: () => void }) {
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const [target, setTarget] = useState(r.target);
  const [saved, setSaved] = useState(r.target);
  useEffect(() => {
    setTarget(r.target);
    setSaved(r.target);
  }, [r.target]);
  return (
    <div className="flex items-center gap-2 border-b bg-card/40 px-4 py-2.5 last:border-0">
      <Avatar name={r.hosts[0]} />
      <a
        href={`https://${r.hosts[0]}`}
        target="_blank"
        rel="noreferrer"
        title={`visitar https://${r.hosts[0]}`}
        className="shrink-0 rounded p-0.5 text-muted-foreground hover:text-foreground"
      >
        <ExternalLink className="h-3.5 w-3.5" />
      </a>
      <span className="font-mono text-xs">{r.hosts.join(", ")}</span>
      <TLSBadge host={r.hosts[0]} />
      <CopyButton text={r.hosts[0]} />
      <span className="text-muted-foreground">→</span>
      {canWrite ? (
        <Input
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          className="min-w-0 flex-1 border-transparent bg-transparent px-1 font-mono text-xs text-muted-foreground hover:border-input focus-visible:border-ring focus-visible:bg-background"
        />
      ) : (
        <span className="font-mono text-xs text-muted-foreground">{r.target}</span>
      )}
      <span className="flex-1" />
      {canWrite && (
        <>
          {target !== saved && (
            <Button
              variant="ghost" size="sm"
              className="text-xs text-emerald-300 hover:text-emerald-100"
              onClick={() => putRoute(r.hosts[0], target.trim()).then(onChanged).catch(fail)}
            >
              guardar
            </Button>
          )}
          <Button
            variant="ghost" size="sm"
            className="text-xs text-muted-foreground hover:text-red-300"
            onClick={() => delRoute(r.hosts[0]).then(onChanged).catch(fail)}
          >
            borrar
          </Button>
        </>
      )}
    </div>
  );
}

export function Routes() {
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const [routes, setRoutes] = useState<Route[]>([]);
  const [hosts, setHosts] = useState("");
  const [target, setTarget] = useState("");
  const [mapLinks, setMapLinks] = useState<GraphLink[]>([]);
  const newHostRef = useRef<HTMLInputElement>(null);

  async function load() {
    try {
      const [rs, px] = await Promise.all([
        listRoutes(),
        fetchProxy().catch(() => null),
      ]);
      setRoutes(rs);
      if (px) {
        setMapLinks(buildLinks(px));
      } else {
        // sin /proxy (p.ej. permiso lectura parcial): grafo honesto sin estado
        setMapLinks(
          rs.flatMap((r) =>
            r.hosts.map((h) => ({
              host: h,
              target: r.target,
              short: r.target.replace(/^https?:\/\//, "").replace(/^127\.0\.0\.1/, "local"),
              ok: true,
              tls: "off",
              stale: true,
            })),
          ),
        );
      }
    } catch (e) {
      fail(e);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="space-y-3">
      <PageHeader
        title="Domains"
        description="Host → backend con TLS real. Editar el target recarga el proxy al instante."
      />
      <FunnelMap links={mapLinks} onAdd={() => newHostRef.current?.focus()} />
      <div className="overflow-hidden rounded-lg border">
        {routes.map((r) => (
          <RouteRow key={r.hosts.join(",") + r.target} r={r} onChanged={load} />
        ))}
        {routes.length === 0 && (
          <EmptyState title="Sin routes" hint="Agrega tu primer dominio abajo." />
        )}
      </div>
      {canWrite && (
        <Section title="Nueva ruta" description="Alta inmediata: el proxy recarga al instante. TLS se emite al primer request.">
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input value={hosts} onChange={(e) => setHosts(e.target.value)} placeholder="host1, host2"
              ref={newHostRef}
              className="flex-1 font-mono text-xs" />
            <Input value={target} onChange={(e) => setTarget(e.target.value)} placeholder="http://127.0.0.1:3000"
              className="tnum flex-1 font-mono text-xs" />
            <Button
              variant="outline" size="sm"
              onClick={() => {
                addRoute(hosts.split(",").map((s) => s.trim()).filter(Boolean), target.trim())
                  .then(() => {
                    setHosts("");
                    setTarget("");
                    load();
                  }).catch(fail);
              }}
            >
              agregar
            </Button>
          </div>
        </Section>
      )}
    </div>
  );
}
