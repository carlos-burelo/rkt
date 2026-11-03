import { useState } from "react";
import { setup } from "./api";
import { Me } from "../../shared/lib/types";

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
    <div className="min-h-screen flex items-center justify-center bg-zinc-950 text-zinc-100">
      <form onSubmit={submit} className="w-80 space-y-3 rounded-lg border border-zinc-800 bg-zinc-900 p-6">
        <h1 className="text-lg font-semibold">▲ Bienvenido a rkt</h1>
        <p className="text-sm text-zinc-400">Crea tu administrador. Solo pasa una vez.</p>
        <input
          value={user}
          onChange={(e) => setUser(e.target.value)}
          placeholder="usuario"
          autoComplete="username"
          className="w-full rounded border border-zinc-700 bg-zinc-800 px-2 py-1.5 text-sm outline-none focus:border-zinc-500"
        />
        <input
          type="password"
          value={p1}
          onChange={(e) => setP1(e.target.value)}
          placeholder="password (mín 12)"
          autoComplete="new-password"
          className="w-full rounded border border-zinc-700 bg-zinc-800 px-2 py-1.5 text-sm outline-none focus:border-zinc-500"
        />
        <input
          type="password"
          value={p2}
          onChange={(e) => setP2(e.target.value)}
          placeholder="repite password"
          autoComplete="new-password"
          className="w-full rounded border border-zinc-700 bg-zinc-800 px-2 py-1.5 text-sm outline-none focus:border-zinc-500"
        />
        {err !== "" && <p className="text-sm text-red-400">{err}</p>}
        <button type="submit" className="w-full rounded bg-zinc-100 px-2 py-1.5 text-sm font-medium text-zinc-900 hover:bg-white">
          Crear admin y entrar
        </button>
      </form>
    </div>
  );
}
