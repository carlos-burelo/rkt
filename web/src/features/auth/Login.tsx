import { useState } from "react";
import { login } from "./api";
import { Me } from "../../shared/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";

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
    <div className="min-h-screen flex items-center justify-center">
      <Card className="w-80">
        <CardHeader>
          <CardTitle className="text-lg">▲ rkt</CardTitle>
          <CardDescription>
            Accede con tu usuario (creado por CLI: <span className="font-mono">users add</span>).
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-3">
            <Input value={user} onChange={(e) => setUser(e.target.value)} placeholder="usuario" autoComplete="username" />
            <Input type="password" value={pass} onChange={(e) => setPass(e.target.value)} placeholder="password" autoComplete="current-password" />
            {err !== "" && <p className="text-sm text-destructive">{err}</p>}
            <Button type="submit" className="w-full">Entrar</Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
