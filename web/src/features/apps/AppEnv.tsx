import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { Globe2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Section, EnvSecret } from "../../shared/components/patterns";
import { setEnv, delEnv } from "./api";
import { useEnv } from "./hooks";
import { useGlobalEnv } from "../env/hooks";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

// AppEnv: variables heredadas de la flota (solo lectura, se editan en
// Entorno) y propias del proyecto (sobrescriben a las globales).
export function AppEnv() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const { env, reload } = useEnv(name);
  const global = useGlobalEnv();
  const fail = useUI((u) => u.fail);
  const { canWrite } = useAuth();
  const [key, setKey] = useState("");
  const [val, setVal] = useState("");
  const keys = Object.keys(env);
  const globalKeys = Object.keys(global.env).sort();

  return (
    <div className="space-y-4">
      <Section
        title="Globales"
        description="heredadas por toda la flota · se editan en Entorno"
        actions={
          <div className="flex items-center gap-2">
            <span className="tnum font-mono text-[9px] text-muted-foreground">{globalKeys.length} vars</span>
            <Button variant="ghost" size="xs" onClick={() => navigate("/environment")}>
              editar en Entorno
            </Button>
          </div>
        }
      >
        <div className="border border-border">
          {globalKeys.map((k, i) => {
            const overridden = k in env;
            return (
              <div
                key={k}
                className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-2.5 py-1.5 last:border-0"
              >
                <span className="folio w-5 shrink-0">{String(i + 1).padStart(2, "0")}</span>
                <Globe2 className="h-3 w-3 shrink-0 text-muted-foreground/60" aria-label="global" />
                <span className="w-44 shrink-0 truncate font-mono text-[11px]">{k}</span>
                <EnvSecret value={global.env[k]} />
                <span
                  className={
                    overridden
                      ? "shrink-0 border border-warn/40 px-1 font-mono text-[9px] text-warn"
                      : "shrink-0 font-mono text-[9px] text-muted-foreground/60"
                  }
                  title={overridden ? "este proyecto define su propio valor" : "llega desde la flota"}
                >
                  {overridden ? "anulada por el proyecto" : "heredada"}
                </span>
              </div>
            );
          })}
          {globalKeys.length === 0 && (
            <p className="bg-card/30 px-3 py-4 text-center font-mono text-[10px] text-muted-foreground">
              sin globales · créalas en Entorno
            </p>
          )}
        </div>
      </Section>

      <Section
        title="Del proyecto"
        description="sobrescriben a las globales · solo operador+ · valores enmascarados"
        actions={<span className="tnum font-mono text-[9px] text-muted-foreground">{keys.length} vars</span>}
      >
        <div className="border border-border">
          {keys.map((k, i) => (
            <div
              key={k}
              className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-2.5 py-1.5 last:border-0"
            >
              <span className="folio w-5 shrink-0">{String(i + 1).padStart(2, "0")}</span>
              <span className="w-44 shrink-0 truncate font-mono text-[11px]">{k}</span>
              <EnvSecret value={env[k]} />
              {k in global.env && (
                <span className="shrink-0 border border-warn/40 px-1 font-mono text-[9px] text-warn">anula global</span>
              )}
              {canWrite && (
                <Button
                  variant="destructive"
                  size="xs"
                  onClick={() => delEnv(name, k).then(reload).catch(fail)}
                >
                  borrar
                </Button>
              )}
            </div>
          ))}
          {keys.length === 0 && (
            <p className="bg-card/30 px-3 py-5 text-center font-mono text-[10px] text-muted-foreground">
              sin vars propias · hereda las globales
            </p>
          )}
        </div>

        {canWrite && (
          <div className="mt-3 border border-border bg-secondary/15 p-2.5">
            <p className="micro mb-2">nueva variable del proyecto</p>
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
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setEnv(name, key.trim(), val)
                      .then(() => {
                        setKey("");
                        setVal("");
                        reload();
                      })
                      .catch(fail);
                  }}
                >
                  guardar
                </Button>
              </div>
            </div>
          </div>
        )}
      </Section>
    </div>
  );
}
