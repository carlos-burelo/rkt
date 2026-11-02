import { useState } from "react";
import { setToken, api, Status } from "./api";

export function Login({ onOk }: { onOk: () => void }) {
  const [tok, setTok] = useState("");
  const [err, setErr] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setToken(tok.trim());
    try {
      await api<Status>("/api/v1/status");
      onOk();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "error");
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-zinc-950 text-zinc-100">
      <form onSubmit={submit} className="w-80 space-y-3 rounded-lg border border-zinc-800 bg-zinc-900 p-6">
        <h1 className="text-lg font-semibold">rkt dashboard</h1>
        <p className="text-sm text-zinc-400">Token de operador (dashboard_token del servidor).</p>
        <input
          type="password"
          value={tok}
          onChange={(e) => setTok(e.target.value)}
          placeholder="token"
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
