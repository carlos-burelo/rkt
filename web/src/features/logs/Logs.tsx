import { RefObject } from "react";
import { AppStatus } from "../../shared/lib/types";

export function Logs({
  apps,
  appFilter,
  setAppFilter,
  log,
  follow,
  setFollow,
  refresh,
  logRef,
}: {
  apps: AppStatus[];
  appFilter: string;
  setAppFilter: (s: string) => void;
  log: string;
  follow: boolean;
  setFollow: (b: boolean) => void;
  refresh: () => void;
  logRef: RefObject<HTMLPreElement | null>;
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <select
          value={appFilter}
          onChange={(e) => setAppFilter(e.target.value)}
          className="rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none"
        >
          <option value="">operativo (rkt.log)</option>
          {apps.map((a) => (
            <option key={a.name} value={a.name}>
              {a.name}
            </option>
          ))}
        </select>
        <label className="flex items-center gap-1.5 text-xs text-zinc-400">
          <input type="checkbox" checked={follow} onChange={(e) => setFollow(e.target.checked)} />
          seguir
        </label>
        <button onClick={refresh} className="rounded-md border border-zinc-800 px-2 py-1 text-xs text-zinc-400 hover:text-zinc-100">
          recargar
        </button>
      </div>
      <pre ref={logRef} className="h-[60vh] overflow-auto rounded-lg border border-zinc-800 bg-black p-3 font-mono text-xs leading-relaxed text-zinc-300">
        {log || "sin líneas"}
      </pre>
    </div>
  );
}
