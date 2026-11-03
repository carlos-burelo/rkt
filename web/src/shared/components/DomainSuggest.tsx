import { useEffect, useState } from "react";
import { suggestDomains, DomainSuggestion } from "../../features/routes/api";

// DomainSuggest: chips de dominios libres para un nombre. Prioriza
// *.localhost (resuelve sin hosts) y el dominio base si está configurado.
export function DomainSuggest({
  name,
  onPick,
  exclude = [],
}: {
  name: string;
  onPick: (host: string) => void;
  exclude?: string[];
}) {
  const [items, setItems] = useState<DomainSuggestion[]>([]);

  useEffect(() => {
    const clean = name.trim().toLowerCase();
    if (clean === "") {
      setItems([]);
      return;
    }
    const t = window.setTimeout(() => {
      suggestDomains(clean)
        .then((s) => setItems(s.filter((x) => !exclude.includes(x.host))))
        .catch(() => setItems([]));
    }, 250);
    return () => window.clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name]);

  if (items.length === 0) return null;

  return (
    <span className="flex flex-wrap items-center gap-1.5">
      <span className="font-mono text-[9px] tracking-[0.08em] text-muted-foreground/70 uppercase">sugeridos</span>
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
    </span>
  );
}
