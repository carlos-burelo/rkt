import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { dur } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { Section, Time } from "../../shared/components/patterns";
import { fetchAnalytics, Analytics } from "../overview/api";
import { useFleet } from "../overview/hooks";
import { useEnvCount } from "./hooks";

function Check({ ok, label }: { ok: boolean; label: string }) {
  return (
    <p className="flex items-center gap-2 py-1 text-xs">
      <span className={`inline-block h-2 w-2 rounded-full ${ok ? "bg-emerald-400" : "bg-zinc-700"}`} />
      <span className={ok ? "text-zinc-300" : "text-zinc-500"}>{label}</span>
    </p>
  );
}

export function AppOverview() {
  const { name = "" } = useParams();
  const { metas, deps, status } = useFleet();
  const [stats, setStats] = useState<Analytics | null>(null);

  const meta = metas.find((m) => m.name === name);
  const st = status?.apps.find((a) => a.name === name);
  const appDeps = deps.filter((d) => d.app === name);
  const prod = appDeps.find((d) => d.status === "ok");
  const domains = meta?.domains ?? [];
  const running = st ? st.running && !st.failed : false;
  const envCount = useEnvCount(name);

  useEffect(() => {
    if (domains[0]) {
      fetchAnalytics(domains[0], 7).then(setStats).catch(() => setStats(null));
    } else {
      setStats(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name, domains.join(",")]);

  return (
    <div className="space-y-4">
      <Section
        title="Production Deployment"
        description={prod ? `rev ${prod.rev.slice(0, 8)} · ${dur(prod.started_at, prod.finished_at)}` : undefined}
      >
        {!prod && <p className="text-xs text-muted-foreground">Sin deploy ok todavía. Publica con Redeploy.</p>}
        {prod && (
          <div className="space-y-1.5">
            <p className="flex items-center gap-2 font-mono text-xs">
              <Dot status="ok" />
              {prod.rev.slice(0, 8)}
              {prod.triggered_by ? <span className="text-muted-foreground">por {prod.triggered_by}</span> : null}
              <span className="flex-1" />
              <Time iso={prod.finished_at} />
            </p>
            {domains.length === 0 && <p className="text-xs text-muted-foreground">Sin dominios — asigna uno en Domains.</p>}
            {domains.map((h) => (
              <p key={h} className="font-mono text-xs">
                <a href={`https://${h}`} target="_blank" rel="noreferrer" className="hover:underline">
                  https://{h} ⧉
                </a>
              </p>
            ))}
          </div>
        )}
      </Section>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Section title="Checklist">
          <Check ok={!!meta?.deploy?.repo} label="Repo conectado" />
          <Check ok={domains.length > 0} label="Dominio asignado" />
          <Check ok={!!prod} label="Primer deploy ok" />
          <Check ok={envCount > 0} label="Env configurado" />
          <Check ok={running} label="App corriendo" />
        </Section>
        <Section title="Tráfico 7d" description={domains[0]}>
          {!stats && <p className="text-xs text-muted-foreground">Sin datos (¿tráfico hacia {domains[0] ?? "…"}?).</p>}
          {stats && (
            <div className="grid grid-cols-2 gap-3 font-mono">
              <div><p className="text-xl tabular-nums">{stats.total}</p><p className="text-[11px] text-muted-foreground">reqs</p></div>
              <div><p className="text-xl tabular-nums">{(stats.bytes / 1048576).toFixed(1)}M</p><p className="text-[11px] text-muted-foreground">bytes</p></div>
              <div><p className="text-xl tabular-nums">{stats.p95 || "—"}</p><p className="text-[11px] text-muted-foreground">p95</p></div>
              <div><p className="text-xl tabular-nums">{stats.by_class["5xx"] ?? 0}</p><p className="text-[11px] text-muted-foreground">5xx</p></div>
            </div>
          )}
        </Section>
      </div>
    </div>
  );
}
