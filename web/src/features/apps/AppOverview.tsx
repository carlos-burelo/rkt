import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { ExternalLink } from "lucide-react";
import { dur } from "../../shared/lib/time";
import { CopyButton } from "../../shared/components/feedback";
import { Section, Stat, Time } from "../../shared/components/patterns";
import { fetchAnalytics, Analytics } from "../overview/api";
import { useFleet } from "../overview/hooks";
import { useEnvCount } from "./hooks";
import { publicUrl, publicHost } from "../../shared/lib/url";
import { AppPreview } from "./AppPreview";

function Check({ ok, label, hint }: { ok: boolean; label: string; hint: string }) {
  return (
    <div className="flex items-center gap-2.5 py-2">
      <span
        aria-hidden
        className={`h-2 w-2 shrink-0 border ${ok ? "border-ok/60 bg-ok" : "border-border bg-transparent"}`}
      />
      <span className="min-w-0 flex-1">
        <span className="block text-[11px]">{label}</span>
        <span className="mt-0.5 block truncate font-mono text-[9px] text-muted-foreground">{hint}</span>
      </span>
      <span className={`font-mono text-[9px] tracking-[0.08em] uppercase ${ok ? "text-ok" : "text-muted-foreground/70"}`}>
        {ok ? "listo" : "pendiente"}
      </span>
    </div>
  );
}

function Cell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0 bg-card/45 px-3 py-2">
      <p className="micro">{label}</p>
      <div className="tnum mt-1 truncate font-mono text-[11px]">{children}</div>
    </div>
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

  if (meta?.external) {
    const cmd = meta.run_command || "pnpm dev";
    return (
      <div className="space-y-4">
        <Section
          title="App externa (solo proxy)"
          description="sin build, sin deploy, sin supervisor · el dev server lo corre tu terminal"
          accent={!running}
        >
          <div className="grid grid-cols-2 gap-px border border-border bg-border sm:grid-cols-4">
            <Cell label="estado">{running ? "escuchando" : "sin backend"}</Cell>
            <Cell label="puerto">{meta.port || "-"}</Cell>
            <Cell label="host">{domains[0] || "-"}</Cell>
            <Cell label="runtime">externo</Cell>
          </div>
          <div className="mt-3 border border-border bg-card/35 px-3 py-2">
            <p className="micro">comando sugerido</p>
            <div className="mt-1 flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate font-mono text-[11px]">{cmd}</code>
              <CopyButton text={cmd} />
            </div>
            <p className="mt-1 font-mono text-[9px] text-muted-foreground">
              correlo en {meta.dir || "la carpeta del repo"}; rkt solo enruta el dominio.
            </p>
          </div>
        </Section>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div
        className={
          domains[0]
            ? "grid grid-cols-1 items-start gap-4 xl:grid-cols-[minmax(320px,auto)_minmax(0,1fr)]"
            : "grid grid-cols-1 gap-4"
        }
      >
        {domains[0] && <AppPreview domain={domains[0]} running={running} />}

        <Section
          title="Despliegue en producción"
          className="h-full"
          description={prod ? `rev ${prod.rev.slice(0, 8)} · ${dur(prod.started_at, prod.finished_at)}` : "sin salida correcta"}
          accent={!prod}
        >
        {!prod && (
          <div className="border border-border bg-card/30 px-4 py-6 text-center">
            <p className="micro text-muted-foreground/60">sin registro</p>
            <p className="mt-2 text-[12px]">Todavía no hay un despliegue correcto.</p>
            <p className="mt-1 font-mono text-[10px] text-muted-foreground">publica con Redesplegar desde la ficha de la unidad.</p>
          </div>
        )}
        {prod && (
          <>
            <div className="grid grid-cols-2 gap-px border border-border bg-border sm:grid-cols-4">
              <Cell label="revisión">
                <span className="flex items-center gap-1">
                  {prod.rev.slice(0, 8)}
                  <CopyButton text={prod.rev} />
                </span>
              </Cell>
              <Cell label="duración">{dur(prod.started_at, prod.finished_at)}</Cell>
              <Cell label="publicó">{prod.triggered_by || "—"}</Cell>
              <Cell label="hora">
                <Time iso={prod.finished_at} />
              </Cell>
            </div>
            <div className="mt-3 border border-border">
              <div className="flex items-center gap-2 border-b border-border bg-secondary/20 px-3 py-1.5">
                <span className="micro">destinos</span>
                <span className="tnum ml-auto font-mono text-[9px] text-muted-foreground">{domains.length}</span>
              </div>
              {domains.length === 0 && (
                <p className="px-3 py-2 font-mono text-[10px] text-muted-foreground">
                  sin dominios — asigna uno en Dominios.
                </p>
              )}
              {domains.map((h) => (
                <a
                  key={h}
                  href={publicUrl(h, status)}
                  target="_blank"
                  rel="noreferrer"
                  className="group flex items-center gap-2 border-b border-border/60 px-3 py-1.5 last:border-0 hover:bg-accent/40"
                >
                  <span aria-hidden className="h-1 w-1 bg-ok" />
                  <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{publicHost(h, status)}</span>
                  <ExternalLink className="h-3 w-3 text-muted-foreground/50 group-hover:text-foreground" />
                </a>
              ))}
            </div>
          </>
        )}
        </Section>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Section title="Checklist de ruta" description="lo que falta para servir">
          <div className="divide-y divide-border/60">
            <Check ok={!!meta?.deploy?.repo} label="Repo conectado" hint="deploy.repo" />
            <Check ok={domains.length > 0} label="Dominio asignado" hint="Dominios → nueva ruta" />
            <Check ok={!!prod} label="Primer despliegue correcto" hint="Redesplegar" />
            <Check ok={envCount > 0} label="Entorno configurado" hint="Entorno" />
            <Check ok={running} label="App en marcha" hint="supervisor" />
          </div>
        </Section>

        <Section title="Tráfico 7d" description={domains[0] ?? "sin dominio"}>
          {!stats && (
            <p className="border border-border bg-card/30 px-3 py-4 font-mono text-[10px] text-muted-foreground">
              sin datos{domains[0] ? ` para ${domains[0]}` : ""} · access.log local
            </p>
          )}
          {stats && (
            <div className="grid grid-cols-2 gap-px border border-border bg-border">
              <Stat value={`${stats.total}`} label="peticiones" sub="ventana 7d" />
              <Stat value={`${(stats.bytes / 1048576).toFixed(1)}M`} label="tráfico" sub="access.log" />
              <Stat value={stats.p95 || "—"} label="p95" sub="latencia" />
              <Stat
                value={`${stats.by_class["5xx"] ?? 0}`}
                label="5xx"
                sub="errores servidor"
                tone={(stats.by_class["5xx"] ?? 0) > 0 ? "warn" : undefined}
              />
            </div>
          )}
        </Section>
      </div>
    </div>
  );
}
