import { useState } from "react";
import { setup } from "./api";
import { Me } from "../../shared/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";

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
    <div className="min-h-screen flex items-center justify-center">
      <Card className="w-80">
        <CardHeader>
          <CardTitle className="text-lg">▲ Bienvenido a rkt</CardTitle>
          <CardDescription>Crea tu administrador. Solo pasa una vez.</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-3">
            <Input value={user} onChange={(e) => setUser(e.target.value)} placeholder="usuario" autoComplete="username" />
            <Input type="password" value={p1} onChange={(e) => setP1(e.target.value)} placeholder="password (mín 12)" autoComplete="new-password" />
            <Input type="password" value={p2} onChange={(e) => setP2(e.target.value)} placeholder="repite password" autoComplete="new-password" />
            {err !== "" && <p className="text-sm text-destructive">{err}</p>}
            <Button type="submit" className="w-full">Crear admin y entrar</Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
