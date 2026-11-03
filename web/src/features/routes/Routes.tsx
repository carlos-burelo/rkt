import { useEffect, useState } from "react";
import { Route } from "../../shared/lib/types";
import { addRoute, delRoute, putRoute, tlsStatus } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";

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

function RouteRow({
  r,
  canWrite,
  refresh,
  fail,
}: {
  r: Route;
  canWrite: boolean;
  refresh: () => void;
  fail: (e: unknown) => void;
}) {
  const [target, setTarget] = useState(r.target);
  return (
    <div className="flex items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-2.5 last:border-0">
      <span className="font-mono text-xs">{r.hosts.join(", ")}</span>
      <TLSBadge host={r.hosts[0]} />
      <span className="text-zinc-600">→</span>
      {canWrite ? (
        <Input
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          className="min-w-0 flex-1 border-transparent bg-transparent px-1 font-mono text-xs text-zinc-400 hover:border-zinc-800 focus-visible:border-zinc-600 focus-visible:bg-zinc-900"
        />
      ) : (
        <span className="font-mono text-xs text-zinc-400">{r.target}</span>
      )}
      <span className="flex-1" />
      {canWrite && (
        <>
          {target !== r.target && (
            <Button
              variant="ghost" size="sm"
              className="text-xs text-emerald-300 hover:text-emerald-100"
              onClick={() => putRoute(r.hosts[0], target.trim()).then(refresh).catch(fail)}
            >
              guardar
            </Button>
          )}
          <Button
            variant="ghost" size="sm"
            className="text-xs text-zinc-600 hover:text-red-300"
            onClick={() => delRoute(r.hosts[0]).then(refresh).catch(fail)}
          >
            borrar
          </Button>
        </>
      )}
    </div>
  );
}
export function Routes({
  routes,
  canWrite,
  refresh,
  fail,
}: {
  routes: Route[];
  canWrite: boolean;
  refresh: () => void;
  fail: (e: unknown) => void;
}) {
  const [hosts, setHosts] = useState("");
  const [target, setTarget] = useState("");

  return (
    <div className="space-y-3">
      <div className="overflow-hidden rounded-lg border border-zinc-800">
        {routes.map((r) => (
          <RouteRow key={r.hosts.join(",") + r.target} r={r} canWrite={canWrite} refresh={refresh} fail={fail} />
        ))}
        {routes.length === 0 && <p className="p-6 text-sm text-zinc-500">Sin routes.</p>}
      </div>
      {canWrite && (
        <div className="flex gap-2">
          <Input value={hosts} onChange={(e) => setHosts(e.target.value)} placeholder="host1, host2"
            className="flex-1 font-mono text-xs" />
          <Input value={target} onChange={(e) => setTarget(e.target.value)} placeholder="http://127.0.0.1:3000"
            className="flex-1 font-mono text-xs" />
          <Button
            variant="outline" size="sm"
            onClick={() => {
              addRoute(hosts.split(",").map((s) => s.trim()).filter(Boolean), target.trim())
                .then(() => {
                  setHosts("");
                  setTarget("");
                  refresh();
                }).catch(fail);
            }}
          >
            agregar
          </Button>
        </div>
      )}
    </div>
  );
}
