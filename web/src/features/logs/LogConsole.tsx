import { useEffect, useRef } from "react";
import { CopyButton } from "../../shared/components/feedback";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import { accessSummary, rejectedSummary, streamOf, type ParsedLine } from "./format";
import { useLogs } from "./logs.store";
import { FieldBit, Highlight, ROW_TONE, LEVEL_KEY, ago } from "./logparts";
import type { RenderedLine } from "./useLogLines";

// Fila de consola: resumen por corriente + bits + copiar. Sin índice,
// sin tag: el nivel colorea la línea.
function LogRow({ p, count }: { p: ParsedLine; count: number }) {
  const { t } = useI18n();
  const wrap = useLogs((s) => s.wrap);
  const absolute = useLogs((s) => s.absolute);
  const compact = useLogs((s) => s.compact);
  const q = useLogs((s) => s.q);
  const appFilter = useUI((s) => s.appFilter);
  const lv = p.level;
  const st = streamOf(p, appFilter);
  const inline = compact && p.structured;
  const hostBit = st === "access" ? p.fields.find((f) => f.key === "host") : undefined;
  const rest = inline
    ? p.fields.filter((f) =>
        st === "access"
          ? !["host", "method", "m", "path", "p", "target", "dur", "duration", "duration_ms", "latency", "latency_ms"].includes(f.key)
          : st === "rejected"
            ? false
            : true,
      )
    : [];
  const main =
    !inline
      ? p.raw
      : st === "access"
        ? (accessSummary(p) ?? p.msg ?? p.raw)
        : st === "rejected"
          ? (rejectedSummary(p) ?? p.msg ?? p.raw)
          : p.msg || p.raw;
  return (
    <div
      title={p.raw}
      aria-label={t("ops.logs.lineLevel", { level: t(LEVEL_KEY[lv]) })}
      className={`flex items-center gap-2 px-3 py-0.5 hover:bg-console-accent ${ROW_TONE[lv]}`}
    >
      <span className="tnum w-14 shrink-0 self-start text-[9px] text-console-muted">{absolute ? p.time : ago(p.timestamp)}</span>
      <span
        className={`min-w-0 flex-1 font-mono text-[11px] ${
          wrap ? "break-words whitespace-pre-wrap" : "truncate"
        }`}
      >
        <Highlight text={main} q={q} />
        {inline && hostBit && hostBit.value !== "" && (
          <>
            {" "}
            <FieldBit fkey="host" value={hostBit.value} />
          </>
        )}
        {rest.slice(0, 6).map((f, idx) => (
          <span key={`${f.key}-${idx}`}>
            {" "}
            <FieldBit fkey={f.key} value={f.value} />
          </span>
        ))}
        {rest.length > 6 && (
          <span
            title={rest.slice(6).map((f) => `${f.key}=${f.value}`).join(" ")}
            className="whitespace-nowrap opacity-50"
          >
            {" "}+{rest.length - 6}
          </span>
        )}
      </span>
      {count > 1 && (
        <span className="tnum shrink-0 font-mono text-[9px] text-console-muted/80">×{count}</span>
      )}
      <CopyButton
        text={p.raw}
        className="text-console-muted/70 hover:bg-console-accent hover:text-console-foreground"
      />
    </div>
  );
}

// Consola con autoscroll: lee líneas ya resueltas, sigue `follow`.
export function LogConsole({ rendered, empty }: { rendered: RenderedLine[]; empty: boolean }) {
  const { t, tn } = useI18n();
  const live = useLogs((s) => s.live);
  const follow = useLogs((s) => s.follow);
  const appFilter = useUI((s) => s.appFilter);
  const logRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (follow && logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [rendered, follow]);

  return (
    <div className="border border-border">
      <div className="flex items-center gap-2 border-b border-console-border bg-console-raised px-3 py-1.5">
        <span aria-hidden className="h-1 w-1 bg-console-ok animate-status-pulse" />
        <span className="micro text-console-muted">{t("ops.logs.console")}</span>
        <span className="flex-1" />
        <span className="tnum font-mono text-[9px] text-console-muted">
          {tn("ops.logs.consoleLines", rendered.length, { app: appFilter || "rkt.log" })}
        </span>
      </div>
      <div
        ref={logRef}
        role="log"
        tabIndex={0}
        aria-live={live ? "polite" : "off"}
        aria-label={t("ops.logs.logAria")}
        className="console h-[58vh] overflow-auto font-mono text-[11px] leading-[1.6]"
      >
        {rendered.map(({ line: p, count }, i) => (
          <LogRow key={i} p={p} count={count} />
        ))}
        {empty && (
          <p className="px-3 py-4 text-console-muted">{t("ops.logs.empty")}</p>
        )}
      </div>
    </div>
  );
}
