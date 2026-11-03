import { useState } from "react";
import { login } from "./api";
import { Me } from "../../shared/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { Avatar } from "../../shared/components/ui";

export function Login({ onOk }: { onOk: (me: Me) => void }) {
  const [user, setUser] = useState("");
  const [pass, setPass] = useState("");
  const [err, setErr] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    try {
      const me = await login(user.trim(), pass);
      onOk(me);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "error");
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-80 overflow-hidden py-0">
        <div aria-hidden className="manifest-strip h-1.5 w-full opacity-80" />
        <CardHeader>
          <div className="flex items-center gap-3">
            <Avatar name="rkt" />
            <div>
              <CardTitle className="text-lg tracking-tight">rkt</CardTitle>
              <p className="tnum font-mono text-[11px] text-muted-foreground">terminal · acceso</p>
            </div>
          </div>
          <CardDescription>
            Accede con tu usuario (creado por CLI: <span className="tnum font-mono">users add</span>).
          </CardDescription>
        </CardHeader>
        <CardContent className="pb-6">
          <form onSubmit={submit} className="space-y-3">
            <Input value={user} onChange={(e) => setUser(e.target.value)} placeholder="usuario" autoComplete="username" className="font-mono text-xs" />
            <Input type="password" value={pass} onChange={(e) => setPass(e.target.value)} placeholder="password" autoComplete="current-password" className="font-mono text-xs" />
            {err !== "" && <p className="text-xs text-destructive">{err}</p>}
            <Button type="submit" className="w-full">Entrar</Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
