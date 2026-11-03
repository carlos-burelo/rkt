import { useEffect, useState } from "react";
import { fetchUsage, Usage } from "./api";
import { getSettings } from "../settings/api";
import { Bars } from "../../shared/components/feedback";

// UsageStrip uso 30d vs quotas (solo visual, sin cobro).
export function UsageStrip() {
  const [u, setU] = useState<Usage | null>(null);
  const [q, setQ] = useState<Record<string, string>>({});

  useEffect(() => {
    fetchUsage(30).then(setU).catch(() => {});
    getSettings().then(setQ).catch(() => {});
  }, []);

  if (!u) return null;
  const qReqs = parseInt(q.quota_reqs || "0", 10);
  const qBytes = parseInt(q.quota_bytes || "0", 10);
  const bars: [string, string, number][] = [
    ["Requests", `${u.total}${qReqs > 0 ? ` / ${qReqs}` : ""}`, qReqs > 0 ? u.total / qReqs : 0],
    ["Bandwidth", `${(u.bytes / 1048576).toFixed(1)}M${qBytes > 0 ? ` / ${(qBytes / 1048576).toFixed(0)}M` : ""}`, qBytes > 0 ? u.bytes / qBytes : 0],
    ["Error rate", `${(u.err_rate * 100).toFixed(1)}%`, u.err_rate],
  ];
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
      <p className="mb-2 text-xs uppercase tracking-wider text-zinc-500">Usage · 30 días</p>
      <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
        {bars.map(([label, v, frac]) => (
          <div key={label}>
            <div className="flex justify-between font-mono text-xs">
              <span className="text-zinc-400">{label}</span>
              <span>{v}</span>
            </div>
            <div className="mt-1 h-1.5 overflow-hidden rounded bg-zinc-800">
              <div
                className={`h-full rounded ${frac > 0.9 ? "bg-red-400" : frac > 0.7 ? "bg-amber-400" : "bg-emerald-400"}`}
                style={{ width: `${Math.min(100, Math.max(2, frac * 100))}%` }}
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
