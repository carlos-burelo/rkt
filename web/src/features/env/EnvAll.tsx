import { useEffect, useState } from "react";
import { listApps, getEnv } from "../apps/api";
import { AppMeta } from "../../shared/lib/types";

// EnvAll vista agrupada por app con buscador global.
export function EnvAll({ fail }: { fail: (e: unknown) => void }) {
  const [q, setQ] = useState("");
  const [rows, setRows] = useState<{ app: string; vars: Record<string, string> }[]>([]);

  useEffect(() => {
    listApps()
      .then(async (apps: AppMeta[]) => {
        const out: { app: string; vars: Record<string, string> }[] = [];
        for (const a of apps) {
          try {
            out.push({ app: a.name, vars: await getEnv(a.name) });
          } catch (e) {
            fail(e);
            return;
          }
        }
        setRows(out);
      })
      .catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const f = q.toLowerCase();
  return (
    <div className="space-y-3">
      <input
        value={q}
        onChange={(e) => setQ(e.target.value)}
        placeholder="Buscar variable o app…"
        className="w-full max-w-md rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1.5 text-xs outline-none placeholder:text-zinc-600"
      />
      {rows.map(({ app, vars }) => {
        const keys = Object.keys(vars).filter(
          (k) => f === "" || k.toLowerCase().includes(f) || app.toLowerCase().includes(f),
        );
        if (f !== "" && keys.length === 0) return null;
        return (
          <div key={app} className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
            <p className="mb-2 font-mono text-xs text-zinc-300">
              {app} <span className="text-zinc-600">{Object.keys(vars).length}</span>
            </p>
            {keys.map((k) => (
              <p key={k} className="flex gap-3 py-0.5 font-mono text-xs">
                <span className="w-56 truncate text-zinc-200">{k}</span>
                <span className="truncate text-zinc-500">🔒 secreto</span>
              </p>
            ))}
            {keys.length === 0 && <p className="text-xs text-zinc-600">Sin vars.</p>}
          </div>
        );
      })}
    </div>
  );
}
