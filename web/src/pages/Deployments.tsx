import { Deployment } from "../api";
import { rel } from "../time";
import { Dot } from "../components/ui";

export function Deployments({
  deps,
  filtered,
  onSelect,
}: {
  deps: Deployment[];
  filtered: boolean;
  onSelect: (d: Deployment) => void;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-zinc-800">
      {deps.map((d) => (
        <button
          key={d.id}
          onClick={() => onSelect(d)}
          className="flex w-full items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-3 text-left last:border-0 hover:bg-zinc-800/40"
        >
          <Dot status={d.status} />
          <span className="font-mono">{d.app}</span>
          <span className="font-mono text-xs text-zinc-500">{d.rev.slice(0, 8)}</span>
          <span className="flex-1" />
          <span className="text-xs text-zinc-500">
            {d.status === "running" ? "building…" : rel(d.finished_at || d.started_at)}
          </span>
          <span className="font-mono text-xs text-zinc-600">#{d.id}</span>
        </button>
      ))}
      {deps.length === 0 && <p className="p-6 text-sm text-zinc-500">Sin deployments{filtered ? " para este filtro" : ""}.</p>}
    </div>
  );
}
