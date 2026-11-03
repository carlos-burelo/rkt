import { useState } from "react";
import { Deployment } from "../../shared/lib/types";
import { rel } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { CopyButton } from "../../shared/components/feedback";

export type DepStatus = "all" | "ok" | "running" | "failed";

export function Deployments({
  deps,
  filtered,
  onSelect,
}: {
  deps: Deployment[];
  filtered: boolean;
  onSelect: (d: Deployment) => void;
}) {
  const [st, setSt] = useState<DepStatus>("all");
  const vis = st === "all" ? deps : deps.filter((d) => d.status === st);
  return (
    <div className="space-y-2">
      <div className="flex gap-1.5">
        {(["all", "ok", "running", "failed"] as const).map((s) => (
          <button
            key={s}
            onClick={() => setSt(s)}
            className={`rounded-full border px-2.5 py-1 font-mono text-[11px] ${
              st === s ? "border-zinc-100 bg-zinc-100 text-zinc-900" : "border-zinc-800 text-zinc-400 hover:text-zinc-200"
            }`}
          >
            {s === "all" ? "todos" : s}
          </button>
        ))}
      </div>
      <div className="overflow-hidden rounded-lg border border-zinc-800">
        {vis.map((d) => (
          <button
            key={d.id}
            onClick={() => onSelect(d)}
            className="flex w-full items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-3 text-left last:border-0 hover:bg-zinc-800/40"
          >
            <Dot status={d.status} />
            <span className="font-mono">{d.app}</span>
            <span className="font-mono text-xs text-zinc-500">{d.rev.slice(0, 8)}</span>
            <CopyButton text={d.rev} />
            <span className="flex-1" />
            <span className="text-xs text-zinc-500">
              {d.status === "running" ? "building…" : rel(d.finished_at || d.started_at)}
            </span>
            <span className="font-mono text-xs text-zinc-600">#{d.id}</span>
          </button>
        ))}
        {vis.length === 0 && <p className="p-6 text-sm text-zinc-500">Sin deployments{filtered || st !== "all" ? " para este filtro" : ""}.</p>}
      </div>
    </div>
  );
}
