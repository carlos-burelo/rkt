import { Deployments } from "../deployments/Deployments";
import { PageHeader } from "../../shared/components/patterns";

// CI pipeline vivo: en curso primero, luego últimos.
export function CI() {
  return (
    <div className="space-y-4">
      <PageHeader title="CI" description="Pipeline vivo: en curso primero, luego últimos. Click abre build log." />
      <div>
        <p className="mb-2 text-xs uppercase tracking-wider text-muted-foreground">En curso</p>
        <Deployments bare status="running" />
      </div>
      <div>
        <p className="mb-2 text-xs uppercase tracking-wider text-muted-foreground">Últimos</p>
        <Deployments bare max={20} />
      </div>
    </div>
  );
}
