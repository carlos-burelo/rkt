import { useState } from "react";
import { useParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Section, EnvSecret } from "../../shared/components/patterns";
import { setEnv, delEnv } from "./api";
import { useEnv } from "./hooks";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

export function AppEnv() {
  const { name = "" } = useParams();
  const { env, reload } = useEnv(name);
  const fail = useUI((u) => u.fail);
  const { canWrite } = useAuth();
  const [key, setKey] = useState("");
  const [val, setVal] = useState("");
  const keys = Object.keys(env);

  return (
    <Section
      title="Environment Variables"
      description="solo operator+ · valores enmascarados · aplicar exige restart"
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
            {canWrite && (
              <Button
                variant="ghost"
                size="xs"
                className="text-muted-foreground hover:text-bad"
                onClick={() => delEnv(name, k).then(reload).catch(fail)}
              >
                borrar
              </Button>
            )}
          </div>
        ))}
        {keys.length === 0 && (
          <p className="bg-card/30 px-3 py-5 text-center font-mono text-[10px] text-muted-foreground">
            sin vars inline · las del sistema viven fuera de esta lista
          </p>
        )}
      </div>

      {canWrite && (
        <div className="mt-3 border border-border bg-secondary/15 p-2.5">
          <p className="micro mb-2">nueva variable</p>
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
  );
}
