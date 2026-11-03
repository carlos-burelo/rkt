// Tiempo relativo corto (es). Sin deps.
export function rel(iso: string): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return iso;
  const s = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (s < 60) return `hace ${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `hace ${m}min`;
  const h = Math.floor(m / 60);
  if (h < 24) return `hace ${h}h`;
  const d = Math.floor(h / 24);
  return `hace ${d}d`;
}

export function dur(startIso: string, endIso: string): string {
  const a = new Date(startIso).getTime();
  const b = endIso ? new Date(endIso).getTime() : Date.now();
  if (Number.isNaN(a) || Number.isNaN(b)) return "—";
  const s = Math.max(0, Math.floor((b - a) / 1000));
  if (s < 60) return `${s}s`;
  return `${Math.floor(s / 60)}min ${s % 60}s`;
}
