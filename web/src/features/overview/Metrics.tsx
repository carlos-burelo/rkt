import { AppStatus, Deployment } from "../../shared/lib/types";

// Metrics tarjetas overview (cómputo local, sin backend).
export function Metrics({ apps, deps }: { apps: AppStatus[]; deps: Deployment[] }) {
  const running = apps.filter((a) => a.running && !a.failed).length;
  const failed = apps.filter((a) => a.failed).length;
  const day = Date.now() - 24 * 3600 * 1000;
  const recent = deps.filter((d) => new Date(d.started_at).getTime() >= day);
  const done = recent.filter((d) => d.status !== "running");
  const ok = done.filter((d) => d.status === "ok").length;
  const rate = done.length === 0 ? "—" : `${Math.round((ok / done.length) * 100)}%`;

  const cards: [string, string][] = [
    [`${running}/${apps.length}`, "apps running"],
    [failed === 0 ? "0" : `${failed}`, "failed"],
    [`${recent.length}`, "deploys 24h"],
    [rate, "tasa ok 24h"],
  ];
  return (
    <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
      {cards.map(([v, label]) => (
        <div key={label} className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
          <p className="font-mono text-xl">{v}</p>
          <p className="mt-1 text-xs text-zinc-500">{label}</p>
        </div>
      ))}
    </div>
  );
}
