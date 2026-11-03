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

  return (
    <Section
      title="Environment Variables"
      description="Solo operator+. Los valores viven enmascarados; aplicar exige restart."
    >
      {Object.entries(env).map(([k]) => (
        <div key={k} className="flex items-center gap-2 py-1">
          <span className="w-48 shrink-0 truncate font-mono text-xs">{k}</span>
          <EnvSecret value={env[k]} />
          {canWrite && (
            <Button variant="ghost" size="sm" className="text-xs text-muted-foreground hover:text-red-300"
              onClick={() => delEnv(name, k).then(reload).catch(fail)}>
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
            className="flex-1 font-mono text-xs" />
          <Button
            variant="outline" size="sm"
            onClick={() => {
              setEnv(name, key.trim(), val).then(() => {
                setKey("");
                setVal("");
                reload();
              }).catch(fail);
            }}
          >
            guardar
          </Button>
        </div>
      )}
    </Section>
  );
}
