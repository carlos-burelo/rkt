import { useMemo } from "react";
import { useUI } from "../../app/ui";
import { groupParsed, parseLogLine, streamOf, type ParsedLine } from "./format";
import { useLogs } from "./logs.store";

export interface LineBuckets {
  vis: ParsedLine[];
  parsed: ParsedLine[];
  counts: Record<"info" | "warning" | "error", number>;
  streams: Record<"access" | "rejected" | "system" | "app", number>;
  buckets: number[];
  stamped: number;
}

export interface RenderedLine {
  line: ParsedLine;
  count: number;
}

// Derivado del log crudo: parseo + filtros + agrupado. Se calcula una
// sola vez aquí; toolbar y consola lo consumen por props.
export function useLogLines(): { lines: LineBuckets; rendered: RenderedLine[] } {
  const log = useLogs((s) => s.log);
  const q = useLogs((s) => s.q);
  const levels = useLogs((s) => s.levels);
  const stream = useLogs((s) => s.stream);
  const compact = useLogs((s) => s.compact);
  const context = useLogs((s) => s.context);
  const appFilter = useUI((s) => s.appFilter);

  const lines = useMemo<LineBuckets>(() => {
    const all = typeof log === "string" && log !== "" ? log.split("\n") : [];
    const parsed = all.map(parseLogLine);
    const counts = { info: 0, warning: 0, error: 0 } as LineBuckets["counts"];
    const streams = { access: 0, rejected: 0, system: 0, app: 0 } as LineBuckets["streams"];
    for (const p of parsed) {
      counts[p.level]++;
      streams[streamOf(p, appFilter)]++;
    }
    const rx = q.trim() === "" ? null : new RegExp(q.trim().replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "i");
    const vis = parsed.filter(
      (p) =>
        levels[p.level] &&
        (stream === "all" || streamOf(p, appFilter) === stream) &&
        (!rx || rx.test(p.raw)),
    );
    const buckets = new Array<number>(24).fill(0);
    let stamped = 0;
    const nowH = Date.now();
    for (const p of parsed) {
      if (!p.timestamp) continue;
      const h = Math.floor((nowH - p.timestamp) / 3600000);
      if (h >= 0 && h < 24) {
        buckets[23 - h]++;
        stamped++;
      }
    }
    return { vis, parsed, counts, streams, buckets, stamped };
  }, [log, q, levels, stream, appFilter]);

  const rendered = useMemo<RenderedLine[]>(() => {
    let list = lines.vis;
    if (context && q.trim() !== "") {
      const rx = new RegExp(q.trim().replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "i");
      const inStream = (p: ParsedLine) => stream === "all" || streamOf(p, appFilter) === stream;
      const keep = new Set<number>();
      lines.parsed.forEach((p, i) => {
        if (!levels[p.level] || !inStream(p) || !rx.test(p.raw)) return;
        for (let j = Math.max(0, i - 2); j <= Math.min(lines.parsed.length - 1, i + 2); j++) keep.add(j);
      });
      list = lines.parsed.filter((p, i) => keep.has(i) && levels[p.level] && inStream(p));
    }
    return compact ? groupParsed(list) : list.map((line) => ({ line, count: 1 }));
  }, [compact, context, lines, q, levels, stream, appFilter]);

  return { lines, rendered };
}
