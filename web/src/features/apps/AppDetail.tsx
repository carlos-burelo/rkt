import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import { ExternalLink } from "lucide-react";
import { dur } from "../../shared/lib/time";
import { Dot, StatusBadge, stateOf } from "../../shared/components/ui";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { PageHeader, Section, Time, EnvSecret } from "../../shared/components/patterns";
import { setEnv, delEnv, updateApp, deleteApp } from "./api";
import { useEnv } from "./hooks";
import { fetchAnalytics, Analytics } from "../overview/api";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

function Check({ ok, label }: { ok: boolean; label: string }) {
  return (
    <p className="flex items-center gap-2 py-1 text-xs">
      <Dot status={ok ? "ok" : "stopped"} />
      <span className={ok ? "text-foreground" : "text-muted-foreground"}>{label}</span>
    </p>
  );
}

export function AppDetail() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const app = name;
  const { metas, deps, status } = useFleet();
  const { env, reload: reloadEnv } = useEnv(app);
  const act = useUI((u) => u.act);
  const fail = useUI((u) => u.fail);
  const setSel = useUI((u) => u.setSel);
  const { canWrite } = useAuth();
  const [key, setKey] = useState("");
  const [val, setVal] = useState("");
  const [form, setForm] = useState({ command: "", dir: "", restart_delay: "", min_uptime: "", repo: "", ref: "", auto: false });
  const [confirmDel, setConfirmDel] = useState("");
  const [stats, setStats] = useState<Analytics | null>(null);

  const meta = metas.find((m) => m.name === app);
  const st = status?.apps.find((a) => a.name === app);
  const appDeps = deps.filter((d) => d.app === app);
  const prod = appDeps.find((d) => d.status === "ok");
  const domains = meta?.domains ?? [];
  const fleetState = st ? stateOf(st) : "stopped";

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

  async function saveSettings() {
    try {
      await updateApp(app, {
        command: form.command || undefined,
        dir: form.dir || undefined,
        restart_delay: form.restart_delay || undefined,
        min_uptime: form.min_uptime || undefined,
        deploy: form.repo
          ? { repo: form.repo, ref: form.ref || "main", build: meta?.deploy?.build ?? [], auto: form.auto }
          : null,
      });
    } catch (e) {
      fail(e);
    }
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title={app || "(sin app)"}
        description={
          st
            ? `${fleetState} · ${st.restarts} restarts${meta ? ` · ${meta.command}` : ""}`
            : undefined
        }
        actions={
          <>
            <StatusBadge status={fleetState} />
            {domains[0] && (
              <a
                href={`https://${domains[0]}`}
                target="_blank"
                rel="noreferrer"
                className="inline-flex h-8 items-center gap-2 whitespace-nowrap rounded-md border border-input bg-background px-3 text-xs font-medium hover:bg-accent hover:text-accent-foreground"
              >
                <ExternalLink className="h-3.5 w-3.5" /> Visit
              </a>
            )}
            {canWrite && app && (
              <>
                <Button variant="outline" size="sm" onClick={() => act(app, "restart")}>Restart</Button>
                <Button variant="outline" size="sm" onClick={() => act(app, "deploy")}>Redeploy</Button>
                <Button variant="outline" size="sm" onClick={() => act(app, "rollback")}>Rollback</Button>
              </>
            )}
          </>
        }
      />

      <Section
        title="Production Deployment"
        description={prod ? `rev ${prod.rev.slice(0, 8)} · ${dur(prod.started_at, prod.finished_at)}` : undefined}
      >
        {!prod && <p className="text-xs text-muted-foreground">Sin deploy ok todavía. Publica con Redeploy.</p>}
        {prod && (
          <div className="space-y-1.5">
            {domains.length === 0 && <p className="text-xs text-muted-foreground">Sin dominios — asigna uno en Domains.</p>}
            {domains.map((h) => (
              <p key={h} className="font-mono text-xs">
                <a href={`https://${h}`} target="_blank" rel="noreferrer" className="hover:underline">
                  https://{h}
                </a>
              </p>
            ))}
            <p className="tnum font-mono text-[11px] text-muted-foreground">
              {prod.triggered_by ? `por ${prod.triggered_by} · ` : ""}<Time iso={prod.finished_at} />
            </p>
          </div>
        )}
      </Section>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Section title="Checklist">
          <Check ok={!!meta?.deploy?.repo} label="Repo conectado" />
          <Check ok={domains.length > 0} label="Dominio asignado" />
          <Check ok={!!prod} label="Primer deploy ok" />
          <Check ok={Object.keys(env).length > 0} label="Env configurado" />
          <Check ok={fleetState === "running"} label="App corriendo" />
        </Section>
        <Section title="Tráfico 7d" description={domains[0]}>
          {!stats && <p className="text-xs text-muted-foreground">Sin datos (¿tráfico hacia {domains[0] ?? "…"}?).</p>}
          {stats && (
            <div className="tnum grid grid-cols-2 gap-3 font-mono">
              <div><p className="text-xl tabular-nums">{stats.total}</p><p className="font-sans text-[11px] text-muted-foreground">reqs</p></div>
              <div><p className="text-xl tabular-nums">{(stats.bytes / 1048576).toFixed(1)}M</p><p className="font-sans text-[11px] text-muted-foreground">bytes</p></div>
              <div><p className="text-xl tabular-nums">{stats.p95 || "—"}</p><p className="font-sans text-[11px] text-muted-foreground">p95</p></div>
              <div><p className="text-xl tabular-nums">{stats.by_class["5xx"] ?? 0}</p><p className="font-sans text-[11px] text-muted-foreground">5xx</p></div>
            </div>
          )}
        </Section>
      </div>

      <Section
        title="Settings"
        description="Guardar valida y aplica (restart requerido para runtime)."
        actions={
          canWrite ? (
            <Button variant="outline" size="sm" onClick={saveSettings}>
              Guardar
            </Button>
          ) : undefined
        }
      >
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
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
              <span className="font-mono text-[11px] text-muted-foreground">{label}</span>
              <Input value={v} onChange={(e) => set(e.target.value)} disabled={!canWrite} className="tnum mt-1 font-mono text-xs" />
            </label>
          ))}
          {canWrite && (
            <label className="flex items-center gap-2 text-xs text-muted-foreground">
              <Checkbox checked={form.auto} onCheckedChange={(v) => setForm({ ...form, auto: v === true })} />
              auto-deploy
            </label>
          )}
        </div>
      </Section>

      <Section
        title="Environment Variables"
        description="Solo operator+. Los valores viven enmascarados; aplicar exige restart."
      >
        {Object.entries(env).map(([k]) => (
          <div key={k} className="flex items-center gap-2 py-1">
            <span className="w-48 shrink-0 truncate font-mono text-xs">{k}</span>
            <EnvSecret value={env[k]} />
            {canWrite && (
              <Button variant="ghost" size="sm" className="text-xs text-muted-foreground hover:text-destructive"
                onClick={() => delEnv(app, k).then(reloadEnv).catch(fail)}>
                borrar
              </Button>
            )}
          </div>
        ))}
        {Object.keys(env).length === 0 && <p className="text-xs text-muted-foreground">Sin vars inline.</p>}
        {canWrite && (
          <div className="mt-3 flex gap-2">
            <Input value={key} onChange={(e) => setKey(e.target.value)} placeholder="CLAVE"
              className="w-40 font-mono text-xs" />
            <Input value={val} onChange={(e) => setVal(e.target.value)} placeholder="valor (admite ${VAR})"
              className="tnum flex-1 font-mono text-xs" />
            <Button
              variant="outline" size="sm"
              onClick={() => {
                setEnv(app, key.trim(), val).then(() => {
                  setKey("");
                  setVal("");
                  reloadEnv();
                }).catch(fail);
              }}
            >
              guardar
            </Button>
          </div>
        )}
      </Section>

      <Section title="Deployments" description="Últimos 10. Click abre build log y timeline.">
        <div className="overflow-hidden rounded-lg border">
          {appDeps.slice(0, 10).map((d) => (
            <button key={d.id} onClick={() => setSel(d)}
              className="flex w-full items-center gap-3 border-b bg-card/40 px-4 py-2.5 text-left last:border-0 hover:bg-accent/40">
              <Dot status={d.status} />
              <span className="tnum font-mono text-xs text-muted-foreground">{d.rev.slice(0, 8)}</span>
              {d.triggered_by ? <span className="tnum hidden font-mono text-[11px] text-muted-foreground sm:block">por {d.triggered_by}</span> : null}
              <span className="flex-1" />
              <span className="tnum text-xs text-muted-foreground"><Time iso={d.finished_at || d.started_at} /></span>
              <span className="tnum font-mono text-xs text-muted-foreground">#{d.id}</span>
            </button>
          ))}
          {appDeps.length === 0 && <p className="p-4 text-xs text-muted-foreground">Sin deployments.</p>}
        </div>
      </Section>

      {canWrite && (
        <div className="pt-4" aria-label="zona de peligro">
          <Section title="Danger zone" description="Borrar quita la app de la flota. Las routes quedan huérfanas: revísalas en Domains.">
            <div className="flex gap-2">
              <Input value={confirmDel} onChange={(e) => setConfirmDel(e.target.value)}
                placeholder={`escribe ${app} para borrar`}
                className="flex-1 border-destructive/50 font-mono text-xs" />
              <Button
                variant="destructive" size="sm"
                disabled={confirmDel !== app}
                onClick={() => {
                  deleteApp(app).then(() => navigate("/deployments")).catch(fail);
                }}
              >
                borrar app
              </Button>
            </div>
          </Section>
        </div>
      )}
    </div>
  );
}
