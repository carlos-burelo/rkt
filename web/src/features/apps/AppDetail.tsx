import { useEffect, useState } from "react";
import { AppMeta, Deployment, AppStatus } from "../../shared/lib/types";
import { dur } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { setEnv, delEnv, updateApp, deleteApp } from "./api";
import { fetchAnalytics, Analytics } from "../overview/api";

function Check({ ok, label }: { ok: boolean; label: string }) {
  return (
    <p className="flex items-center gap-2 py-1 text-xs">
      <span className={`inline-block h-2 w-2 rounded-full ${ok ? "bg-emerald-400" : "bg-zinc-700"}`} />
      <span className={ok ? "text-zinc-300" : "text-zinc-500"}>{label}</span>
    </p>
  );
}

export function AppDetail({
  app,
  meta,
  status,
  env,
  deployments,
  canWrite,
  onAction,
  onOpenDeploy,
  onDeleted,
  refresh,
  fail,
}: {
  app: string;
  meta: AppMeta | undefined;
  status: AppStatus | undefined;
  env: Record<string, string>;
  deployments: Deployment[];
  canWrite: boolean;
  onAction: (app: string, a: "restart" | "deploy" | "rollback") => void;
  onOpenDeploy: (d: Deployment) => void;
  onDeleted: () => void;
  refresh: () => void;
  fail: (e: unknown) => void;
}) {
  const [key, setKey] = useState("");
  const [val, setVal] = useState("");
  const [form, setForm] = useState({ command: "", dir: "", restart_delay: "", min_uptime: "", repo: "", ref: "", auto: false });
  const [confirmDel, setConfirmDel] = useState("");
  const [stats, setStats] = useState<Analytics | null>(null);

  const prod = deployments.find((d) => d.status === "ok");
  const domains = meta?.domains ?? [];
  const running = status ? status.running && !status.failed : false;

  useEffect(() => {
    setForm({
      command: meta?.command ?? "",
      dir: meta?.dir ?? "",
      restart_delay: meta?.restart_delay ?? "",
      min_uptime: meta?.min_uptime ?? "",
      repo: meta?.deploy?.repo ?? "",
      ref: meta?.deploy?.ref ?? "",
      auto: meta?.deploy?.auto ?? false,
    });
  }, [meta]);

  useEffect(() => {
    if (domains[0]) {
      fetchAnalytics(domains[0], 7).then(setStats).catch(() => setStats(null));
    } else {
      setStats(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [app, domains.join(",")]);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <h2 className="font-mono text-lg">{app || "(sin app)"}</h2>
        <Dot status={running ? "running" : "stopped"} />
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

      {/* production deployment */}
      <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
        <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">Production Deployment</p>
        {!prod && <p className="text-xs text-zinc-500">Sin deploy ok todavía.</p>}
        {prod && (
          <div className="space-y-1 font-mono text-xs">
            <p>
              {prod.app} · rev {prod.rev.slice(0, 8)} · {dur(prod.started_at, prod.finished_at)}
              {prod.triggered_by ? <span className="text-zinc-500"> · por {prod.triggered_by}</span> : null}
            </p>
            {domains.map((h) => (
              <p key={h}>
                <a href={`https://${h}`} target="_blank" rel="noreferrer" className="text-zinc-200 hover:underline">
                  https://{h} ⧉
                </a>
              </p>
            ))}
            {domains.length === 0 && <p className="text-zinc-500">Sin dominios (agrega en Domains).</p>}
          </div>
        )}
      </div>

      {/* checklist */}
      <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
        <p className="mb-1 text-xs uppercase tracking-wider text-zinc-500">Production Checklist</p>
        <Check ok={!!meta?.deploy?.repo} label="Repo conectado" />
        <Check ok={domains.length > 0} label="Dominio asignado" />
        <Check ok={!!prod} label="Primer deploy ok" />
        <Check ok={Object.keys(env).length > 0} label="Env configurado" />
        <Check ok={running} label="App corriendo" />
      </div>

      {/* tráfico 7d */}
      {stats && (
        <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
          <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">Tráfico 7d · {domains[0]}</p>
          <div className="grid grid-cols-2 gap-2 font-mono text-xs md:grid-cols-4">
            <div><p className="text-lg text-zinc-100">{stats.total}</p><p className="text-zinc-500">reqs</p></div>
            <div><p className="text-lg text-zinc-100">{(stats.bytes / 1048576).toFixed(1)}M</p><p className="text-zinc-500">bytes</p></div>
            <div><p className="text-lg text-zinc-100">{stats.p95 || "—"}</p><p className="text-zinc-500">p95</p></div>
            <div><p className="text-lg text-zinc-100">{stats.by_class["5xx"] ?? 0}</p><p className="text-zinc-500">5xx</p></div>
          </div>
        </div>
      )}

      {/* settings */}
      <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
        <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">Settings</p>
        <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
          {(
            [
              ["command", form.command, (v: string) => setForm({ ...form, command: v })],
              ["dir", form.dir, (v: string) => setForm({ ...form, dir: v })],
              ["restart_delay", form.restart_delay, (v: string) => setForm({ ...form, restart_delay: v })],
              ["min_uptime", form.min_uptime, (v: string) => setForm({ ...form, min_uptime: v })],
              ["repo", form.repo, (v: string) => setForm({ ...form, repo: v })],
              ["ref", form.ref, (v: string) => setForm({ ...form, ref: v })],
            ] as const
          ).map(([label, v, set]) => (
            <label key={label} className="block">
              <span className="font-mono text-[11px] text-zinc-500">{label}</span>
              <input
                value={v}
                onChange={(e) => set(e.target.value)}
                disabled={!canWrite}
                className="mt-0.5 w-full rounded border border-zinc-700 bg-zinc-900 px-2 py-1 font-mono text-xs outline-none disabled:text-zinc-500"
              />
            </label>
          ))}
          {canWrite && (
            <label className="flex items-center gap-2 text-xs text-zinc-400">
              <input type="checkbox" checked={form.auto} onChange={(e) => setForm({ ...form, auto: e.target.checked })} />
              auto-deploy
            </label>
          )}
        </div>
        {canWrite && (
          <button
            onClick={() => {
              updateApp(app, {
                command: form.command || undefined,
                dir: form.dir || undefined,
                restart_delay: form.restart_delay || undefined,
                min_uptime: form.min_uptime || undefined,
                deploy: form.repo
                  ? { repo: form.repo, ref: form.ref || "main", build: meta?.deploy?.build ?? [], auto: form.auto }
                  : null,
              }).then(refresh).catch(fail);
            }}
            className="mt-3 rounded border border-zinc-700 px-3 py-1.5 text-xs hover:bg-zinc-800"
          >
            guardar (restart requerido)
          </button>
        )}
      </div>

      {/* env */}
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

      {/* deploys */}
      <div className="overflow-hidden rounded-lg border border-zinc-800">
        {deployments.slice(0, 10).map((d) => (
          <button key={d.id} onClick={() => onOpenDeploy(d)}
            className="flex w-full items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-2.5 text-left last:border-0 hover:bg-zinc-800/40">
            <Dot status={d.status} />
            <span className="font-mono text-xs text-zinc-400">{d.rev.slice(0, 8)}</span>
            {d.triggered_by ? <span className="font-mono text-[11px] text-zinc-600">por {d.triggered_by}</span> : null}
            <span className="flex-1" />
            <span className="text-xs text-zinc-500">{dur(d.started_at, d.finished_at)}</span>
            <span className="font-mono text-xs text-zinc-600">#{d.id}</span>
          </button>
        ))}
      </div>

      {/* danger */}
      {canWrite && (
        <div className="rounded-lg border border-red-900 bg-red-950/20 p-4">
          <p className="mb-2 text-xs uppercase tracking-wider text-red-400">Danger zone</p>
          <div className="flex gap-2">
            <input value={confirmDel} onChange={(e) => setConfirmDel(e.target.value)}
              placeholder={`escribe ${app} para borrar`}
              className="flex-1 rounded border border-red-900 bg-zinc-900 px-2 py-1 font-mono text-xs outline-none" />
            <button
              disabled={confirmDel !== app}
              onClick={() => {
                deleteApp(app).then(() => onDeleted()).catch(fail);
              }}
              className="rounded border border-red-900 px-3 py-1 text-xs text-red-300 hover:bg-red-950 disabled:opacity-40"
            >
              borrar app
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
