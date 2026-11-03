import { Deployments } from "../deployments/Deployments";
import { PageHeader, Stat, StatGrid } from "../../shared/components/patterns";
import { useFleet } from "../overview/hooks";

// CI pipeline vivo: en curso primero, luego últimos. Tablero de salidas,
// misma gramática que Deployments (columnas fijas, duraciones tabulares).
export function CI() {
  const { deps } = useFleet();
  const running = deps.filter((d) => d.status === "running").length;
  const ok = deps.filter((d) => d.status === "ok").length;
  const failed = deps.filter((d) => d.status === "failed").length;

  return (
    <div className="space-y-5">
      <PageHeader
        title="CI"
        description="Canal vivo: en curso primero, luego los últimos. Click abre el manifiesto de la compilación."
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{deps.length} salidas</span>}
      />
      <StatGrid>
        <Stat value={`${running}`} label="en curso" sub="ahora" tone={running > 0 ? "signal" : undefined} />
        <Stat value={`${ok}`} label="ok" sub="histórico visible" tone={ok > 0 ? "ok" : undefined} />
        <Stat value={`${failed}`} label="fallidos" sub="requieren mirada" tone={failed > 0 ? "bad" : undefined} />
        <Stat value={`${deps.length}`} label="salidas" sub="solo producción" />
      </StatGrid>

      <section aria-label="en curso">
        <div className="mb-2 flex items-end justify-between gap-3">
          <div>
            <h3 className="text-[13px] font-medium tracking-tight">En curso</h3>
            <p className="mt-0.5 font-mono text-[9px] text-muted-foreground">estado · salida · rev · dur · hora</p>
          </div>
          <span className="tnum font-mono text-[9px] text-muted-foreground">{running} activos</span>
        </div>
        <Deployments bare status="running" />
      </section>

      <section aria-label="últimos">
        <div className="mb-2 flex items-end justify-between gap-3">
          <div>
            <h3 className="text-[13px] font-medium tracking-tight">Últimas salidas</h3>
            <p className="mt-0.5 font-mono text-[9px] text-muted-foreground">ventana 20 · solo producción</p>
          </div>
          <span className="tnum font-mono text-[9px] text-muted-foreground">{Math.min(20, deps.length)} registros</span>
        </div>
        <Deployments bare max={20} />
      </section>
    </div>
  );
}
