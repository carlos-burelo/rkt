import { useState } from "react";
import { login } from "./api";
import { Me } from "../../shared/lib/types";

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
    <div className="min-h-screen flex items-center justify-center bg-zinc-950 text-zinc-100">
      <form onSubmit={submit} className="w-80 space-y-3 rounded-lg border border-zinc-800 bg-zinc-900 p-6">
        <h1 className="text-lg font-semibold">▲ rkt</h1>
        <p className="text-sm text-zinc-400">Accede con tu usuario (creado por CLI: <span className="font-mono">users add</span>).</p>
        <input
          value={user}
          onChange={(e) => setUser(e.target.value)}
          placeholder="usuario"
          autoComplete="username"
          className="w-full rounded border border-zinc-700 bg-zinc-800 px-2 py-1.5 text-sm outline-none focus:border-zinc-500"
        />
        <input
          type="password"
          value={pass}
          onChange={(e) => setPass(e.target.value)}
          placeholder="password"
          autoComplete="current-password"
          className="w-full rounded border border-zinc-700 bg-zinc-800 px-2 py-1.5 text-sm outline-none focus:border-zinc-500"
        />
        {err !== "" && <p className="text-sm text-red-400">{err}</p>}
        <button type="submit" className="w-full rounded bg-zinc-100 px-2 py-1.5 text-sm font-medium text-zinc-900 hover:bg-white">
          Entrar
        </button>
      </form>
    </div>
  );
}
