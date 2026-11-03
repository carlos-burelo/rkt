import { useState } from "react";
import { AppMeta, Deployment } from "../../shared/lib/types";
import { dur } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { setEnv, delEnv } from "./api";

export function AppDetail({
  app,
  meta,
  env,
  deployments,
  canWrite,
  onAction,
  onOpenDeploy,
  refresh,
  fail,
}: {
  app: string;
  meta: AppMeta | undefined;
  env: Record<string, string>;
  deployments: Deployment[];
  canWrite: boolean;
  onAction: (app: string, a: "restart" | "deploy" | "rollback") => void;
  onOpenDeploy: (d: Deployment) => void;
  refresh: () => void;
  fail: (e: unknown) => void;
}) {
  const [key, setKey] = useState("");
  const [val, setVal] = useState("");

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <h2 className="font-mono text-lg">{app || "(sin app)"}</h2>
        <span className="flex-1" />
        {canWrite && app && (
          <div className="flex gap-2">
            {(["restart", "deploy", "rollback"] as const).map((a) => (
              <button
                key={a}
                onClick={() => onAction(app, a)}
                className="rounded-md border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800"
              >
                {a}
              </button>
            ))}
          </div>
        )}
      </div>
      {meta && (
        <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4 font-mono text-xs">
          <p className="text-zinc-400">{meta.command} {(meta.args ?? []).join(" ")}</p>
          <p className="mt-1 text-zinc-500">{meta.dir}</p>
          {meta.deploy && (
            <p className="mt-1 text-zinc-500">
              {meta.deploy.repo} @ {meta.deploy.ref}
              {meta.deploy.auto ? " · auto" : ""}
            </p>
          )}
        </div>
      )}
      <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
        <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">Environment (restart requerido)</p>
        {Object.entries(env).map(([k, v]) => (
          <div key={k} className="flex items-center gap-2 py-1 font-mono text-xs">
            <span className="w-48 truncate text-zinc-200">{k}</span>
            <span className="flex-1 truncate text-zinc-500">{v}</span>
            {canWrite && (
              <button onClick={() => delEnv(app, k).then(refresh).catch(fail)} className="text-zinc-600 hover:text-red-300">
                borrar
              </button>
            )}
          </div>
        ))}
        {Object.keys(env).length === 0 && <p className="text-xs text-zinc-600">Sin vars inline.</p>}
        {canWrite && (
          <div className="mt-2 flex gap-2">
            <input value={key} onChange={(e) => setKey(e.target.value)} placeholder="CLAVE"
              className="w-40 rounded border border-zinc-700 bg-zinc-900 px-2 py-1 font-mono text-xs outline-none" />
            <input value={val} onChange={(e) => setVal(e.target.value)} placeholder="valor"
              className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1 font-mono text-xs outline-none" />
            <button
              onClick={() => {
                setEnv(app, key.trim(), val).then(() => {
                  setKey("");
                  setVal("");
                  refresh();
                }).catch(fail);
              }}
              className="rounded border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800"
            >
              guardar
            </button>
          </div>
        )}
      </div>
      <div className="overflow-hidden rounded-lg border border-zinc-800">
        {deployments.slice(0, 10).map((d) => (
          <button key={d.id} onClick={() => onOpenDeploy(d)}
            className="flex w-full items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-2.5 text-left last:border-0 hover:bg-zinc-800/40">
            <Dot status={d.status} />
            <span className="font-mono text-xs text-zinc-400">{d.rev.slice(0, 8)}</span>
            <span className="flex-1" />
            <span className="text-xs text-zinc-500">{dur(d.started_at, d.finished_at)}</span>
            <span className="font-mono text-xs text-zinc-600">#{d.id}</span>
          </button>
        ))}
      </div>
    </div>
  );
}
