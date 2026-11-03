import { AppStatus, Deployment } from "../../shared/lib/types";
import { Stat } from "../../shared/components/patterns";

// Placas de flota a escala fija: mismo tamaño, número tabular arriba,
// etiqueta abajo, sub honesto con ventana de medición.
export function Metrics({ apps, deps }: { apps: AppStatus[]; deps: Deployment[] }) {
  const running = apps.filter((a) => a.running && !a.failed).length;
  const failed = apps.filter((a) => a.failed).length;
  const day = Date.now() - 24 * 3600 * 1000;
  const recent = deps.filter((d) => new Date(d.started_at).getTime() >= day);
  const done = recent.filter((d) => d.status !== "running");
  const ok = done.filter((d) => d.status === "ok").length;
  const rate = done.length === 0 ? "—" : `${Math.round((ok / done.length) * 100)}%`;

  return (
    <div className="grid grid-cols-2 gap-3 md:grid-cols-4" role="group" aria-label="estado de la flota">
      <Stat value={`${running}/${apps.length}`} label="apps en ruta" sub="supervisor · ahora" tone={failed > 0 ? "warn" : "ok"} />
      <Stat
        value={failed === 0 ? "0" : `${failed}`}
        label="varadas"
        sub={failed === 0 ? "sin fallos" : "requieren acción"}
        tone={failed === 0 ? undefined : "bad"}
      />
      <Stat value={`${recent.length}`} label="deploys 24h" sub="flota · ventana 24h" tone="signal" />
      <Stat value={rate} label="tasa ok 24h" sub={done.length === 0 ? "sin deploys" : `${ok}/${done.length} ok`} />
    </div>
  );
}
