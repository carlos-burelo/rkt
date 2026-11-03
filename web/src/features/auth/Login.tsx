import { useState } from "react";
import { login } from "./api";
import { Me } from "../../shared/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Err } from "../../shared/components/ui";
import { AccessShell } from "./AccessShell";

export function Login({ onOk }: { onOk: (me: Me) => void }) {
  const [user, setUser] = useState("");
  const [pass, setPass] = useState("");
  const [err, setErr] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    try {
      const me = await login(user.trim(), pass);
      onOk(me);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "error");
    }
  }

  return (
    <AccessShell
      kicker="acceso"
      title="Inicia sesión"
      description="Entra con tu cuenta de rkt. Las cuentas se crean desde la terminal; aquí no hay registro."
      footer={
        <p className="font-mono text-[9px] leading-relaxed text-muted-foreground/70">
          ¿Sin cuenta? Créala con <span className="text-foreground/80">rkt users add</span>
          <br />
          permisos: lectura · operador · administrador
        </p>
      }
    >
      <form onSubmit={submit} className="space-y-3">
        <Err msg={err} />
        <label className="field-label">
          usuario
          <Input
            value={user}
            onChange={(e) => setUser(e.target.value)}
            placeholder="tu usuario"
            autoComplete="username"
            className="h-8 font-mono text-[12px]"
          />
        </label>
        <label className="field-label">
          contraseña
          <Input
            type="password"
            value={pass}
            onChange={(e) => setPass(e.target.value)}
            placeholder="tu contraseña"
            autoComplete="current-password"
            className="tnum h-8 font-mono text-[12px]"
          />
        </label>
        <Button type="submit" className="mt-1 h-8 w-full justify-between">
          <span>Entrar</span>
          <span aria-hidden className="font-mono text-[9px] tracking-[0.08em] opacity-60">↵</span>
        </Button>
      </form>
    </AccessShell>
  );
}
