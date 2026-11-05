import { useEffect, useState } from "react";
import { ArrowRight, ExternalLink, Plus } from "lucide-react";
import { Route, Status } from "../../shared/lib/types";
import { addRoute, delRoute, putRoute, tlsStatus, listRoutes } from "./api";
import { fetchProxy } from "../proxy/api";
import { buildLinks, GraphLink } from "../proxy/graph";
import { FunnelMap } from "./FunnelMap";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { CopyButton } from "../../shared/components/feedback";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { Avatar } from "../../shared/components/ui";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { LocalZones } from "./LocalZones";
import { useFleet } from "../overview/hooks";
import { publicUrl, publicHost } from "../../shared/lib/url";

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
    <span
      title={txt}
      className={`inline-flex h-5 w-fit items-center border px-1.5 font-mono text-[10px] tracking-[0.08em] uppercase ${
        ok ? "border-ok/30 bg-ok/10 text-ok" : "border-border bg-transparent text-muted-foreground"
      }`}
    >
      {ok ? `tls ${txt}` : "sin tls"}
    </span>
  );
}

function RouteRow({ r, onChanged, status }: { r: Route; onChanged: () => void; status: Status | null }) {
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const [target, setTarget] = useState(r.target);
  const [edit, setEdit] = useState(false);
  useEffect(() => {
    setTarget(r.target);
  }, [r.target]);
  const save = async () => {
    await putRoute(r.hosts[0], target.trim());
    onChanged();
  };
  return (
    <div className="group relative grid grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 px-3 py-2.5 last:border-0 hover:bg-accent/30 md:grid-cols-[1.5rem_minmax(0,1.4fr)_auto_1.5rem_minmax(0,1.4fr)_auto]">
      <Avatar name={r.hosts[0]} />
      <div className="min-w-0">
        <div className="flex items-center gap-1.5">
          <a
            href={publicUrl(r.hosts[0], status)}
            target="_blank"
            rel="noreferrer"
            title={`visitar ${publicHost(r.hosts[0], status)}`}
            className="truncate font-mono text-[12px] hover:text-signal hover:underline"
          >
            {r.hosts.join(", ")}
          </a>
          <CopyButton text={r.hosts[0]} />
          <ExternalLink className="h-3 w-3 shrink-0 text-muted-foreground/45" />
        </div>
        <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground md:hidden">→ {r.target}</p>
      </div>
      <TLSBadge host={r.hosts[0]} />
      <ArrowRight className="hidden h-3 w-3 text-muted-foreground/30 md:block" />
      <span className="tnum col-span-2 min-w-0 truncate font-mono text-[11px] text-muted-foreground md:col-span-1">
        {r.target}
      </span>
      {canWrite && (
        <div className="col-span-3 flex justify-end gap-1 md:col-span-1">
          <FormDialog
            open={edit}
            onOpenChange={setEdit}
            trigger={
              <Button variant="ghost" size="xs">
                editar
              </Button>
            }
            title={`Target de ${r.hosts[0]}`}
            description="el proxy recarga al instante · solo loopback"
            submitLabel="guardar"
            canSubmit={target.trim() !== "" && target.trim() !== r.target}
            onSubmit={save}
          >
            <Field label="target" hint="http(s)://127.0.0.1:puerto">
              <Input
                value={target}
                onChange={(e) => setTarget(e.target.value)}
                aria-label={`target de ${r.hosts[0]}`}
                className="tnum h-8 font-mono text-[12px]"
                autoFocus
              />
            </Field>
          </FormDialog>
          <ConfirmButton
            title={`borrar ruta ${r.hosts[0]}`}
            onConfirm={() => {
              delRoute(r.hosts[0]).then(onChanged).catch(fail);
            }}
          />
        </div>
      )}
    </div>
  );
}

export function Routes() {
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const { status } = useFleet();
  const [routes, setRoutes] = useState<Route[]>([]);
  const [hosts, setHosts] = useState("");
  const [target, setTarget] = useState("");
  const [mapLinks, setMapLinks] = useState<GraphLink[]>([]);
  const [addOpen, setAddOpen] = useState(false);

  async function load() {
    try {
      const [rs, px] = await Promise.all([listRoutes(), fetchProxy().catch(() => null)]);
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

  const hostCount = routes.reduce((n, r) => n + r.hosts.length, 0);

  return (
    <div className="space-y-4">
      <PageHeader
        title="Dominios"
        description="Host → backend con TLS real. Editar el target recarga el proxy al instante. Sin wildcards inventados."
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {routes.length} rutas · {hostCount} hosts
          </span>
        }
        actions={
          canWrite ? (
            <FormDialog
              open={addOpen}
              onOpenChange={setAddOpen}
              trigger={
                <Button size="sm">
                  <Plus className="h-3 w-3" /> nueva ruta
                </Button>
              }
              title="Nueva ruta"
              description="alta inmediata · el proxy recarga al instante · TLS se emite al primer request"
              submitLabel="crear"
              canSubmit={hosts.trim() !== "" && target.trim() !== ""}
              onSubmit={async () => {
                await addRoute(
                  hosts.split(",").map((s) => s.trim()).filter(Boolean),
                  target.trim(),
                );
                setHosts("");
                setTarget("");
                await load();
              }}
            >
              <Field label="hosts" hint="separados por coma">
                <Input
                  value={hosts}
                  onChange={(e) => setHosts(e.target.value)}
                  placeholder="app.localhost, app.test"
                  className="h-8 font-mono text-[12px]"
                  autoFocus
                />
              </Field>
              <Field label="target" hint="http(s)://127.0.0.1:puerto">
                <Input
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  placeholder="http://127.0.0.1:3000"
                  className="tnum h-8 font-mono text-[12px]"
                />
              </Field>
            </FormDialog>
          ) : undefined
        }
      />
      <LocalZones onChanged={load} />

      <FunnelMap links={mapLinks} onAdd={() => setAddOpen(true)} />

      <div className="border border-border" role="table" aria-label="rutas de dominio">
        <div
          className="hidden grid-cols-[1.5rem_minmax(0,1.4fr)_auto_1.5rem_minmax(0,1.4fr)_auto] items-center gap-2 border-b border-border bg-secondary/25 px-3 py-1.5 md:grid"
          aria-hidden
        >
          <span className="micro">est</span>
          <span className="micro">host</span>
          <span className="micro">tls</span>
          <span />
          <span className="micro">target</span>
          <span />
        </div>
        {routes.map((r) => (
          <RouteRow key={r.hosts.join(",") + r.target} r={r} onChanged={load} status={status} />
        ))}
        {routes.length === 0 && <EmptyState title="Sin rutas" hint="Agrega tu primer dominio con «nueva ruta»." />}
      </div>
    </div>
  );
}
