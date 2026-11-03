import { useEffect, useState } from "react";
import { Route } from "../../shared/lib/types";
import { addRoute, delRoute, putRoute, tlsStatus } from "./api";

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
      className={`rounded-full px-2 py-0.5 font-mono text-[11px] ${ok ? "bg-emerald-500/15 text-emerald-300" : "bg-zinc-800 text-zinc-500"}`}
      title={txt}
    >
      {ok ? `TLS ${txt}` : "sin TLS"}
    </span>
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
        <input
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          className="min-w-0 flex-1 rounded border border-transparent bg-transparent px-1 font-mono text-xs text-zinc-400 outline-none hover:border-zinc-800 focus:border-zinc-600 focus:bg-zinc-900"
        />
      ) : (
        <span className="font-mono text-xs text-zinc-400">{r.target}</span>
      )}
      <span className="flex-1" />
      {canWrite && (
        <>
          {target !== r.target && (
            <button
              onClick={() => putRoute(r.hosts[0], target.trim()).then(refresh).catch(fail)}
              className="text-xs text-emerald-300 hover:text-emerald-100"
            >
              guardar
            </button>
          )}
          <button
            onClick={() => delRoute(r.hosts[0]).then(refresh).catch(fail)}
            className="text-xs text-zinc-600 hover:text-red-300"
          >
            borrar
          </button>
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
          <input value={hosts} onChange={(e) => setHosts(e.target.value)} placeholder="host1, host2"
            className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
          <input value={target} onChange={(e) => setTarget(e.target.value)} placeholder="http://127.0.0.1:3000"
            className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
          <button
            onClick={() => {
              addRoute(hosts.split(",").map((s) => s.trim()).filter(Boolean), target.trim())
                .then(() => {
                  setHosts("");
                  setTarget("");
                  refresh();
                }).catch(fail);
            }}
            className="rounded border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800"
          >
            agregar
          </button>
        </div>
      )}
    </div>
  );
}
