import { useEffect, useState } from "react";
import { suggestDomains, DomainSuggestResult } from "../../features/routes/api";
import { useI18n } from "../i18n";

const empty: DomainSuggestResult = { suggestions: [], public: false, requires_domain: false };

// DomainSuggest: chips de dominios libres para un nombre. En local prioriza
// *.localhost; en servidor público exige un dominio con acceso o base_domain.
export function DomainSuggest({
  name,
  onPick,
  exclude = [],
}: {
  name: string;
  onPick: (host: string) => void;
  exclude?: string[];
}) {
  const { t } = useI18n();
  const [data, setData] = useState<DomainSuggestResult>(empty);

  useEffect(() => {
    const clean = name.trim().toLowerCase();
    if (clean === "") {
      setData(empty);
      return;
    }
    const timer = window.setTimeout(() => {
      suggestDomains(clean)
        .then(setData)
        .catch(() => setData(empty));
    }, 250);
    return () => window.clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name]);

  const items = data.suggestions.filter((x) => !exclude.includes(x.host));
  if (items.length === 0 && !data.requires_domain) return null;

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {data.requires_domain && (
        <span className="w-full border border-warn/30 bg-warn/5 px-2 py-1 font-mono text-[9px] text-warn">
          {t("shared.domain.requires")}
        </span>
      )}
      {items.length > 0 && (
        <span className="font-mono text-[9px] tracking-[0.08em] text-muted-foreground/70 uppercase">
          {t("shared.domain.suggested")}
        </span>
      )}
      {items.map((s) => (
        <button
          key={s.host}
          type="button"
          onClick={() => onPick(s.host)}
          title={s.note}
          className="inline-flex items-center gap-1 border border-border bg-card/50 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground hover:border-signal/50 hover:text-foreground"
        >
          {s.host}
          <span className="font-mono text-[8px] tracking-[0.08em] uppercase opacity-60">
            {s.kind === "base" ? "base" : s.kind}
          </span>
        </button>
      ))}
    </div>
  );
}
