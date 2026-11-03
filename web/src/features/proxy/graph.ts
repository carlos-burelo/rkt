import type { ProxyRoute } from "./api";

// Grafo canónico proxy: un solo hogar para derivar los dos mapas
// (radial en /proxy, funnel en /routes). Funciones puras y testeables.
export interface GraphLink {
  host: string;
  target: string;
  short: string;
  ok: boolean;
  tls: string;
  /** true cuando el estado no se midió (fallback sin /proxy): no fingir verde */
  stale?: boolean;
}

export function shortTarget(target: string): string {
  const t = target.trim();
  const m = t.match(/^(?:https?:\/\/)?([^/]+)(\/.*)?$/i);
  const host = (m?.[1] ?? t).replace(/^127\.0\.0\.1/, "local").replace(/^localhost/, "local");
  return host || t;
}

export function buildLinks(rows: ProxyRoute[]): GraphLink[] {
  const out: GraphLink[] = [];
  for (const r of rows) {
    for (const h of r.hosts) {
      out.push({
        host: h,
        target: r.target,
        short: shortTarget(r.target),
        ok: r.ok,
        tls: r.tls[0] ?? "off",
      });
    }
  }
  return out.sort((a, b) => a.host.localeCompare(b.host));
}

export function groupByTarget(links: GraphLink[]): { target: string; short: string; hosts: GraphLink[] }[] {
  const m = new Map<string, GraphLink[]>();
  for (const l of links) {
    const arr = m.get(l.target) ?? [];
    arr.push(l);
    m.set(l.target, arr);
  }
  return [...m.entries()]
    .map(([target, hosts]) => ({ target, short: hosts[0]?.short ?? target, hosts }))
    .sort((a, b) => b.hosts.length - a.hosts.length || a.short.localeCompare(b.short));
}

// Posición radial en [0,1]². Órbita elíptica, arranque arriba (-90°).
export function radialPos(i: number, n: number): { x: number; y: number } {
  if (n === 1) return { x: 0.5, y: 0.14 };
  const a = -Math.PI / 2 + (2 * Math.PI * i) / n;
  return { x: 0.5 + 0.34 * Math.cos(a), y: 0.5 + 0.33 * Math.sin(a) };
}
