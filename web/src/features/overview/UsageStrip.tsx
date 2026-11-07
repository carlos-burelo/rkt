import { useEffect } from "react";
import { Meter } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import { useUsageStrip } from "./usage.store";

type Tone = "ok" | "warn" | "bad" | "signal";

function toneOf(frac: number | undefined): Tone {
  if (frac === undefined) return "signal";
  if (frac > 0.9) return "bad";
  if (frac > 0.7) return "warn";
  return "ok";
}

// UsageStrip: uso 30d vs quotas (solo visual, sin cobro). La barra solo
// existe cuando hay tope declarado; sin tope se muestra la cifra honesta.
export function UsageStrip() {
  const { t } = useI18n();
  const u = useUsageStrip((s) => s.u);
  const q = useUsageStrip((s) => s.quotas);

  useEffect(() => {
    useUsageStrip.getState().load();
  }, []);

  if (!u) return <p className="font-mono text-[10px] text-muted-foreground">{t("overview.usage.empty")}</p>;
  const qReqs = parseInt(q.quota_reqs || "0", 10);
  const qBytes = parseInt(q.quota_bytes || "0", 10);
  const reqFrac = qReqs > 0 ? u.total / qReqs : undefined;
  const byteFrac = qBytes > 0 ? u.bytes / qBytes : undefined;
  const bars: { label: string; value: string; frac?: number }[] = [
    { label: t("overview.requests"), value: `${u.total}${qReqs > 0 ? ` / ${qReqs}` : ""}`, frac: reqFrac },
    {
      label: t("overview.usage.bandwidth"),
      value: `${(u.bytes / 1048576).toFixed(1)}M${qBytes > 0 ? ` / ${(qBytes / 1048576).toFixed(0)}M` : ""}`,
      frac: byteFrac,
    },
    { label: t("overview.errorRate"), value: `${(u.err_rate * 100).toFixed(1)}%`, frac: u.err_rate },
  ];
  return (
    <div className="space-y-3">
      {bars.map((b) => (
        <Meter key={b.label} label={b.label} value={b.value} frac={b.frac} tone={toneOf(b.frac)} />
      ))}
      <div className="flex flex-wrap items-baseline justify-between gap-2 border-t border-border/60 pt-2">
        <span className="font-mono text-[9px] text-muted-foreground">{t("overview.usage.measured")}</span>
        <span className="tnum font-mono text-[9px] text-muted-foreground">{t("overview.usage.p95", { value: u.p95 || "—" })}</span>
      </div>
    </div>
  );
}
