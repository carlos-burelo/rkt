import { useState } from "react";
import { setup } from "./api";
import { Me } from "../../shared/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { Avatar } from "../../shared/components/ui";

// Setup primer arranque: crea el admin (one-shot, el backend lo
// permite solo sin usuarios). Sin comandos.
export function Setup({ onOk }: { onOk: (me: Me) => void }) {
  const [user, setUser] = useState("");
  const [p1, setP1] = useState("");
  const [p2, setP2] = useState("");
  const [err, setErr] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (p1 !== p2) {
      setErr("passwords no coinciden");
      return;
    }
    try {
      const me = await setup(user.trim(), p1);
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
              <CardTitle className="text-lg tracking-tight">Bienvenido a rkt</CardTitle>
              <p className="tnum font-mono text-[11px] text-muted-foreground">terminal · alta inicial</p>
            </div>
          </div>
          <CardDescription>Crea tu administrador. Solo pasa una vez.</CardDescription>
        </CardHeader>
        <CardContent className="pb-6">
          <form onSubmit={submit} className="space-y-3">
            <Input value={user} onChange={(e) => setUser(e.target.value)} placeholder="usuario" autoComplete="username" className="font-mono text-xs" />
            <Input type="password" value={p1} onChange={(e) => setP1(e.target.value)} placeholder="password (mín 12)" autoComplete="new-password" className="tnum font-mono text-xs" />
            <Input type="password" value={p2} onChange={(e) => setP2(e.target.value)} placeholder="repite password" autoComplete="new-password" className="tnum font-mono text-xs" />
            {err !== "" && <p className="text-xs text-destructive">{err}</p>}
            <Button type="submit" className="w-full">Crear admin y entrar</Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
