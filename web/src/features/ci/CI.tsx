import { Deployment } from "../../shared/lib/types";
import { Deployments } from "../deployments/Deployments";

// CI pipeline vivo: en curso primero, luego últimos.
export function CI({
  deps,
  onSelect,
}: {
  deps: Deployment[];
  onSelect: (d: Deployment) => void;
}) {
  const running = deps.filter((d) => d.status === "running");
  const done = deps.filter((d) => d.status !== "running").slice(0, 20);
  return (
    <div className="space-y-4">
      <div>
        <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">
          En curso ({running.length})
        </p>
        {running.length === 0 && <p className="text-xs text-zinc-600">Nada corriendo.</p>}
        {running.length > 0 && <Deployments deps={running} filtered={false} onSelect={onSelect} />}
      </div>
      <div>
        <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">Últimos</p>
        <Deployments deps={done} filtered={false} onSelect={onSelect} />
      </div>
    </div>
  );
}
