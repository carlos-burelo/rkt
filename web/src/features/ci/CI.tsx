import { Deployments } from "../deployments/Deployments";
import { PageHeader } from "../../shared/components/patterns";

// CI pipeline vivo: en curso primero, luego últimos. Tablero de salidas,
// misma gramática que Deployments (columnas fijas, duraciones tabulares).
export function CI() {
  return (
    <div className="space-y-4">
      <PageHeader title="CI" description="Pipeline vivo: en curso primero, luego últimos. Click abre build log." />
      <section aria-label="en curso">
        <div className="mb-2 flex items-baseline justify-between">
          <h3 className="text-sm font-medium">En curso</h3>
          <p className="tnum font-mono text-[11px] text-muted-foreground">estado · salida · rev · dur · hora</p>
        </div>
        <Deployments bare status="running" />
      </section>
      <section aria-label="últimos">
        <div className="mb-2 flex items-baseline justify-between">
          <h3 className="text-sm font-medium">Últimas salidas</h3>
          <p className="tnum font-mono text-[11px] text-muted-foreground">ventana 20 · production-only</p>
        </div>
        <Deployments bare max={20} />
      </section>
    </div>
  );
}
