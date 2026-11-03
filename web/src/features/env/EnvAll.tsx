import { useEffect, useState } from "react";
import { ArrowRight, Globe2, Lock, Search } from "lucide-react";
import { useNavigate } from "react-router";
import { listApps, getEnv } from "../apps/api";
import { AppMeta } from "../../shared/lib/types";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Avatar } from "../../shared/components/ui";
import { PageHeader, EmptyState, Section, EnvSecret } from "../../shared/components/patterns";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useGlobalEnv } from "./hooks";
import { delGlobalEnv, setGlobalEnv } from "./api";

// EnvAll: variables globales de la flota + inventario por app.
// Los valores se revelan a demanda; la edición de globales vive aquí.
export function EnvAll() {
  const fail = useUI((u) => u.fail);
  const canWrite = useAuth((a) => a.canWrite);
  const navigate = useNavigate();
  const global = useGlobalEnv();
  const [q, setQ] = useState("");
  const [rows, setRows] = useState<{ app: string; vars: Record<string, string> }[]>([]);
  const [key, setKey] = useState("");
  const [val, setVal] = useState("");

  useEffect(() => {
    listApps()
      .then(async (apps: AppMeta[]) => {
        const out: { app: string; vars: Record<string, string> }[] = [];
        for (const a of apps) {
          try {
            out.push({ app: a.name, vars: await getEnv(a.name) });
          } catch (e) {
            fail(e);
            return;
          }
        }
        setRows(out);
      })
      .catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const f = q.toLowerCase();
  const globalKeys = Object.keys(global.env).sort();
  const ownTotal = rows.reduce((n, r) => n + Object.keys(r.vars).length, 0);
  const visible = rows.filter(({ app, vars }) => {
    if (f === "") return true;
    return app.toLowerCase().includes(f) || Object.keys(vars).some((k) => k.toLowerCase().includes(f));
  });

  const save = () => {
    const k = key.trim();
    if (!k) return;
    setGlobalEnv(k, val)
      .then(() => {
        setKey("");
        setVal("");
        global.reload();
      })
      .catch(fail);
  };

  return (
    <div className="space-y-4">
      <PageHeader
        title="Entorno"
        description="Globales para toda la flota y propias de cada proyecto. Las del proyecto sobrescriben a las globales."
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {globalKeys.length} globales · {ownTotal} propias · {rows.length} apps
          </span>
        }
      />

      <Section
        title="Globales"
        accent
        description="se inyectan en todas las apps · la app puede anular una clave"
        actions={<span className="tnum font-mono text-[9px] text-muted-foreground">{globalKeys.length} vars</span>}
      >
        <div className="border border-border">
          {globalKeys.map((k, i) => (
            <div key={k} className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-2.5 py-1.5 last:border-0">
              <span className="folio w-5 shrink-0">{String(i + 1).padStart(2, "0")}</span>
              <Globe2 className="h-3 w-3 shrink-0 text-muted-foreground/60" aria-label="global" />
              <span className="w-44 shrink-0 truncate font-mono text-[11px]">{k}</span>
              <EnvSecret value={global.env[k]} />
              {canWrite && (
                <Button variant="destructive" size="xs" onClick={() => delGlobalEnv(k).then(global.reload).catch(fail)}>
                  borrar
                </Button>
              )}
            </div>
          ))}
          {globalKeys.length === 0 && (
            <p className="bg-card/30 px-3 py-5 text-center font-mono text-[10px] text-muted-foreground">
              sin globales · agrega la primera abajo
            </p>
          )}
        </div>

        {canWrite && (
          <div className="mt-3 border border-border bg-secondary/15 p-2.5">
            <p className="micro mb-2">nueva global</p>
            <div className="flex flex-col gap-2 sm:flex-row">
              <label className="field-label w-44 shrink-0">
                clave
                <Input
                  value={key}
                  onChange={(e) => setKey(e.target.value)}
                  placeholder="CLAVE"
                  className="h-7 font-mono text-[11px]"
                />
              </label>
              <label className="field-label min-w-0 flex-1">
                valor
                <Input
                  value={val}
                  onChange={(e) => setVal(e.target.value)}
                  placeholder="valor (admite ${VAR})"
                  className="h-7 font-mono text-[11px]"
                />
              </label>
              <div className="flex items-end">
                <Button variant="outline" size="sm" onClick={save}>
                  guardar
                </Button>
              </div>
            </div>
          </div>
        )}
      </Section>

      <div className="flex items-center gap-2 border border-border bg-card/35 p-2">
        <div className="relative min-w-0 flex-1 sm:max-w-md">
          <Search className="absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="buscar variable o app…"
            className="h-7 border-transparent bg-transparent pl-8 font-mono text-[11px] hover:border-input"
          />
        </div>
        <span className="flex-1" />
        <span className="folio">valores ocultos</span>
      </div>

      {visible.map(({ app, vars }) => {
        const keys = Object.keys(vars).filter((k) => f === "" || k.toLowerCase().includes(f) || app.toLowerCase().includes(f));
        const inherited = globalKeys.filter((k) => !(k in vars)).length;
        return (
          <section key={app} className="border border-border">
            <button
              onClick={() => navigate(`/app/${app}`)}
              className="flex w-full items-center gap-2.5 border-b border-border bg-secondary/20 px-3 py-2 text-left hover:bg-accent/40"
            >
              <Avatar name={app} />
              <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{app}</span>
              <span className="tnum font-mono text-[9px] text-muted-foreground">
                {Object.keys(vars).length} propias · {inherited} heredadas
              </span>
              <ArrowRight className="h-3 w-3 text-muted-foreground/40" />
            </button>
            {keys.map((k) => (
              <div key={k} className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-3 py-1.5 last:border-0">
                <Lock className="h-3 w-3 shrink-0 text-muted-foreground/60" aria-label="secreto" />
                <span className="w-52 shrink-0 truncate font-mono text-[11px]">{k}</span>
                {k in global.env && (
                  <span className="border border-warn/40 px-1 font-mono text-[9px] text-warn">anula global</span>
                )}
                <span className="flex-1" />
                <span className="font-mono text-[9px] text-muted-foreground/60">editar en app</span>
              </div>
            ))}
            {keys.length === 0 && (
              <p className="bg-card/30 px-3 py-3 font-mono text-[10px] text-muted-foreground">sin propias para este filtro</p>
            )}
          </section>
        );
      })}
      {rows.length === 0 && <EmptyState title="Sin apps" hint="Registra tu primera app para ver su inventario." />}
      {rows.length > 0 && visible.length === 0 && (
        <EmptyState title="Sin coincidencias" hint={`Ninguna variable o app coincide con “${q}”.`} />
      )}
    </div>
  );
}
