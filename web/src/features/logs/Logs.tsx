import { useEffect, useMemo, useRef, useState } from "react";
import { Clock, Download, Globe, Lock, LockOpen, Pause, Play, Search } from "lucide-react";import { AppStatus, Deployment } from "../../shared/lib/types";
import { deploymentLog } from "../deployments/api";
import { streamLog } from "../../shared/lib/http";
import { fetchLog } from "./api";
import { Bars } from "../../shared/components/feedback";
import { CopyButton } from "../../shared/components/feedback";
import { Dot, Avatar } from "../../shared/components/ui";
import { PageHeader, Section } from "../../shared/components/patterns";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import { groupParsed, parseLogLine, accessSummary, rejectedSummary, streamOf, type Level, type Stream } from "./format";

const LEVEL_TONE: Record<Level, string> = {
  info: "bg-info",
  warning: "bg-warn",
  error: "bg-bad",
};

const LEVEL_TEXT: Record<Level, string> = {
  info: "text-info",
  warning: "text-warn",
  error: "text-bad",
};

const LEVEL_KEY: Record<Level, string> = {
  info: "ops.logs.level.info",
  warning: "ops.logs.level.warning",
  error: "ops.logs.level.error",
};

const LEVEL_SHORT: Record<Level, string> = { info: "INF", warning: "WRN", error: "ERR" };

const STREAM_KEY: Record<Stream, string> = {
  access: "ops.logs.stream.access",
  rejected: "ops.logs.stream.rejected",
  system: "ops.logs.stream.system",
  app: "ops.logs.stream.app",
};

const STREAM_SHORT: Record<Stream, string> = { access: "ACC", rejected: "REJ", system: "SYS", app: "APP" };

const ROW_TONE: Record<Level, string> = {
  info: "text-console-foreground/85",
  warning: "text-console-warn",
  error: "text-console-bad",
};

function shortVal(value: string): string {
  return value.length > 40 ? `${value.slice(0, 40)}…` : value;
}

// Bit compacto con iconografía: candado abierto/cerrado para http/https,
// mundo para host, reloj para duración. Valores vacíos se ocultan
// (ej `https=` sin TLS solo mete ruido). title conserva clave=valor.
function FieldBit({ fkey, value }: { fkey: string; value: string }) {
  if (value === "") return null;
  if (fkey === "https") {
    return (
      <span title={`https=${value}`} className="inline-flex items-center gap-1 whitespace-nowrap">
        <Lock className="h-3 w-3 shrink-0 opacity-70" aria-label="https" /> {shortVal(value)}
      </span>
    );
  }
  if (fkey === "http") {
    return (
      <span title={`http=${value}`} className="inline-flex items-center gap-1 whitespace-nowrap">
        <LockOpen className="h-3 w-3 shrink-0 opacity-70" aria-label="http" /> {shortVal(value)}
      </span>
    );
  }
  if (fkey === "host") {
    return (
      <span title={`host=${value}`} className="inline-flex items-center gap-1 whitespace-nowrap opacity-80">
        <Globe className="h-3 w-3 shrink-0 opacity-70" aria-hidden /> {shortVal(value)}
      </span>
    );
  }
  if (fkey === "dur" || fkey === "duration" || fkey === "duration_ms" || fkey === "latency" || fkey === "latency_ms") {
    return (
      <span title={`${fkey}=${value}`} className="inline-flex items-center gap-1 whitespace-nowrap opacity-70">
        <Clock className="h-3 w-3 shrink-0 opacity-70" aria-hidden /> {shortVal(value)}
      </span>
    );
  }
  if (fkey === "target") {
    return <span title={`target=${value}`} className="whitespace-nowrap opacity-80">→ {shortVal(value)}</span>;
  }
  if (fkey === "remote" || fkey === "remote_addr" || fkey === "ip") {
    return <span title={`${fkey}=${value}`} className="whitespace-nowrap opacity-70">← {shortVal(value)}</span>;
  }
  if (fkey === "method" || fkey === "m") {
    return <span className="whitespace-nowrap font-semibold">{value.toUpperCase()}</span>;
  }
  if (fkey === "status" || fkey === "status_code" || fkey === "st") {
    return <span className="whitespace-nowrap font-semibold">{value}</span>;
  }
  if (fkey === "path" || fkey === "p" || fkey === "query" || fkey === "q") {
    return <span className="whitespace-nowrap">{shortVal(value)}</span>;
  }
  return (
    <span title={`${fkey}=${value}`} className="whitespace-nowrap opacity-60">
      {fkey}={shortVal(value)}
    </span>
  );
}

function Highlight({ text, q }: { text: string; q: string }) {
  const needle = q.trim();
  if (needle === "") return <>{text}</>;
  const rx = new RegExp(needle.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "gi");
  const out: React.ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(rx)) {
    const i = m.index ?? 0;
    if (i > last) out.push(text.slice(last, i));
    out.push(
      <mark key={`${i}-${m[0]}`} className="bg-console-warn/25 text-console-warn">
        {m[0]}
      </mark>,
    );
    last = i + m[0].length;
  }
  if (last < text.length) out.push(text.slice(last));
  return <>{out}</>;
}

function ago(ts?: number): string {
  if (!ts) return "";
  const s = Math.max(0, Math.round((Date.now() - ts) / 1000));
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.round(s / 60)}m`;
  return `${Math.round(s / 3600)}h`;
}

function LevelToggle({
  level,
  on,
  count,
  onClick,
}: {
  level: Level;
  on: boolean;
  count: number;
  onClick: () => void;
}) {
  const { t } = useI18n();
  const label = t(LEVEL_KEY[level]);
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={on}
      aria-label={label}
      title={label}
      className={`inline-flex h-7 items-center gap-1.5 border px-2 font-mono text-[9px] tracking-[0.08em] uppercase transition-colors ${
        on ? "border-input bg-accent/60 text-foreground" : "border-border bg-transparent text-muted-foreground/60 hover:text-foreground"
      }`}
    >
      <span aria-hidden className={`h-1.5 w-1.5 ${on ? LEVEL_TONE[level] : "bg-muted-foreground/25"}`} />
      <span className={on ? LEVEL_TEXT[level] : undefined}>{LEVEL_SHORT[level]}</span>
      <span className={`tnum ${on ? LEVEL_TEXT[level] : "text-muted-foreground/50"}`}>{count}</span>
    </button>
  );
}

function StreamToggle({
  stream,
  on,
  count,
  onClick,
}: {
  stream: Stream;
  on: boolean;
  count: number;
  onClick: () => void;
}) {
  const { t } = useI18n();
  const label = t(STREAM_KEY[stream]);
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={on}
      aria-label={label}
      title={label}
      className={`inline-flex h-7 items-center gap-1.5 border px-2 font-mono text-[9px] tracking-[0.08em] uppercase transition-colors ${
        on ? "border-input bg-accent/60 text-foreground" : "border-border bg-transparent text-muted-foreground/60 hover:text-foreground"
      }`}
    >
      <span className={on ? undefined : "text-muted-foreground/50"}>{STREAM_SHORT[stream]}</span>
      <span className={`tnum ${on ? undefined : "text-muted-foreground/50"}`}>{count}</span>
    </button>
  );
}

export function Logs() {
  const { t, tn } = useI18n();
  const { status, deps } = useFleet();
  const apps: AppStatus[] = status?.apps ?? [];
  const appFilter = useUI((u) => u.appFilter);
  const setAppFilter = useUI((u) => u.setAppFilter);
  const fail = useUI((u) => u.fail);
  const online = useUI((u) => u.online);
  const [tab, setTab] = useState<"runtime" | "build">("runtime");
  const [q, setQ] = useState("");
  const [live, setLive] = useState(true);
  const [levels, setLevels] = useState<Record<Level, boolean>>({ info: true, warning: true, error: true });
  const [stream, setStream] = useState<"all" | Stream>("all");
  const [log, setLog] = useState("");
  const [follow, setFollow] = useState(true);
  const [compact, setCompact] = useState(true);
  const [wrap, setWrap] = useState(false);
  const [absolute, setAbsolute] = useState(true);
  const [context, setContext] = useState(false);
  const [build, setBuild] = useState(t("ops.logs.pickDeployment"));
  const logRef = useRef<HTMLDivElement>(null);
  const latest = deps.find((d: Deployment) => appFilter === "" || d.app === appFilter);

  async function load() {
    try {
      setLog(await fetchLog(appFilter));
    } catch (e) {
      fail(e);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [appFilter, online]);

  useEffect(() => {
    setStream("all");
  }, [appFilter]);

  useEffect(() => {
    if (!live || tab !== "runtime" || !online) return;
    const q = `n=200${appFilter !== "" ? `&app=${encodeURIComponent(appFilter)}` : ""}`;
    return streamLog(`/api/v1/logs/stream?${q}`, (text, append) => {
      if (append) setLog((prev) => prev + text);
      else setLog(text);
    });
  }, [live, tab, appFilter, online]);

  useEffect(() => {
    if (follow && logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [log, follow]);

  const lines = useMemo(() => {
    const all = typeof log === "string" && log !== "" ? log.split("\n") : [];
    const parsed = all.map(parseLogLine);
    const counts: Record<Level, number> = { info: 0, warning: 0, error: 0 };
    const streams: Record<Stream, number> = { access: 0, rejected: 0, system: 0, app: 0 };
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

  const rendered = useMemo(() => {
    let list = lines.vis;
    if (context && q.trim() !== "") {
      const rx = new RegExp(q.trim().replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "i");
      const inStream = (p: (typeof lines.parsed)[number]) => stream === "all" || streamOf(p, appFilter) === stream;
      const keep = new Set<number>();
      lines.parsed.forEach((p, i) => {
        if (!levels[p.level] || !inStream(p) || !rx.test(p.raw)) return;
        for (let j = Math.max(0, i - 2); j <= Math.min(lines.parsed.length - 1, i + 2); j++) keep.add(j);
      });
      list = lines.parsed.filter((p, i) => keep.has(i) && levels[p.level] && inStream(p));
    }
    return compact ? groupParsed(list) : list.map((line) => ({ line, count: 1 }));
  }, [compact, context, lines, q, levels, stream, appFilter]);

  function download() {
    const blob = new Blob([log], { type: "text/plain" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `${appFilter || "rkt"}.log`;
    a.click();
    URL.revokeObjectURL(a.href);
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("ops.logs.title")}
        description={t("ops.logs.description")}
        meta={
          <span className="tnum inline-flex items-center gap-2 font-mono text-[10px] text-muted-foreground">
            <Dot status={tab === "runtime" && live ? "running" : "stopped"} pulse={tab === "runtime" && live} />
            {tab === "runtime"
              ? live
                ? t("ops.logs.meta.live")
                : t("ops.logs.meta.paused")
              : t("ops.logs.meta.build")}
            {appFilter ? ` · ${appFilter}` : t("ops.logs.meta.operational")}
          </span>
        }
      />

      <div className="border border-border bg-card/35">
        <div className="flex flex-wrap items-center gap-2 border-b border-border p-2">
          <Tabs value={tab} onValueChange={(v) => setTab(v as "runtime" | "build")}>
            <TabsList>
              <TabsTrigger value="runtime">{t("ops.logs.tab.runtime")}</TabsTrigger>
              <TabsTrigger value="build">{t("ops.logs.tab.build")}</TabsTrigger>
            </TabsList>
          </Tabs>
          <span aria-hidden className="hidden h-4 w-px bg-border sm:block" />
          {tab === "runtime" && (
            <>
              <Select value={appFilter || "__all"} onValueChange={(v: string | null) => setAppFilter(v === "__all" ? "" : (v ?? ""))}>
                <SelectTrigger className="h-7 w-52 font-mono text-[10px]">
                  <span className="truncate">{appFilter || t("ops.logs.operational")}</span>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all">{t("ops.logs.operational")}</SelectItem>
                  {apps.map((a) => (
                    <SelectItem key={a.name} value={a.name}>
                      {a.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <div className="relative min-w-40 flex-1 sm:max-w-64">
                <Search className="absolute top-1/2 left-2 h-3 w-3 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                  placeholder={t("ops.logs.searchPlaceholder")}
                  className="h-7 pl-7 font-mono text-[10px]"
                />
              </div>
              <div className="flex items-center gap-px" role="group" aria-label={t("ops.logs.levelsAria")}>
                {(["info", "warning", "error"] as const).map((l) => (
                  <LevelToggle
                    key={l}
                    level={l}
                    on={levels[l]}
                    count={lines.counts[l]}
                    onClick={() => setLevels({ ...levels, [l]: !levels[l] })}
                  />
                ))}
              </div>
              {appFilter === "" && (
                <div className="flex items-center gap-px" role="group" aria-label={t("ops.logs.streamsAria")}>
                  <button
                    type="button"
                    onClick={() => setStream("all")}
                    aria-pressed={stream === "all"}
                    aria-label={t("ops.logs.stream.all")}
                    title={t("ops.logs.stream.all")}
                    className={`inline-flex h-7 items-center gap-1.5 border px-2 font-mono text-[9px] tracking-[0.08em] uppercase transition-colors ${
                      stream === "all" ? "border-input bg-accent/60 text-foreground" : "border-border bg-transparent text-muted-foreground/60 hover:text-foreground"
                    }`}
                  >
                    <span className={`tnum ${stream === "all" ? undefined : "text-muted-foreground/50"}`}>
                      {lines.vis.length}
                    </span>
                  </button>
                  {(["access", "rejected", "system"] as const).map((s) => (
                    <StreamToggle
                      key={s}
                      stream={s}
                      on={stream === s}
                      count={lines.streams[s]}
                      onClick={() => setStream(stream === s ? "all" : s)}
                    />
                  ))}
                </div>
              )}
              <span className="flex-1" />
              <Button variant="outline" size="sm" onClick={() => setLive(!live)}>
                {live ? <Pause className="h-3 w-3" /> : <Play className="h-3 w-3" />}
                {live ? t("ops.logs.pause") : t("ops.logs.live")}
              </Button>
              <Button variant="ghost" size="sm" onClick={load}>{t("ops.logs.reload")}</Button>
              <Button variant="ghost" size="sm" onClick={download}>
                <Download className="h-3 w-3" /> {t("ops.logs.download")}
              </Button>
            </>
          )}
        </div>

        {tab === "runtime" && (
          <div className="space-y-3 p-2">
            <div className="flex flex-wrap items-center gap-3">
              <label className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
                <Checkbox checked={follow} onCheckedChange={(v) => setFollow(v === true)} />
                {t("ops.logs.follow")}
              </label>
              <label className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
                <Checkbox checked={compact} onCheckedChange={(v) => setCompact(v === true)} />
                {t("ops.logs.compact")}
              </label>
              <label className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
                <Checkbox checked={wrap} onCheckedChange={(v) => setWrap(v === true)} />
                {t("ops.logs.wrap")}
              </label>
              <label className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
                <Checkbox checked={absolute} onCheckedChange={(v) => setAbsolute(v === true)} />
                {t("ops.logs.absolute")}
              </label>
              {q.trim() !== "" && (
                <label className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
                  <Checkbox checked={context} onCheckedChange={(v) => setContext(v === true)} />
                  {t("ops.logs.context")}
                </label>
              )}
              <span className="tnum font-mono text-[9px] text-muted-foreground">
                {tn("ops.logs.visibleLines", log === "" ? 0 : log.split("\n").length, {
                  vis: lines.vis.length,
                })}
              </span>
            </div>

            {lines.stamped > 0 && (
              <Section
                title={t("ops.logs.activity")}
                description={tn("ops.logs.activityDesc", lines.stamped)}
              >
                <Bars data={lines.buckets} height={44} />
              </Section>
            )}

            <div className="border border-border">
              <div className="flex items-center gap-2 border-b border-console-border bg-console-raised px-3 py-1.5">
                <span aria-hidden className="h-1 w-1 bg-console-ok animate-status-pulse" />
                <span className="micro text-console-muted">{t("ops.logs.console")}</span>
                <span className="flex-1" />
                <span className="tnum font-mono text-[9px] text-console-muted">
                  {tn("ops.logs.consoleLines", lines.vis.length, { app: appFilter || "rkt.log" })}
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
                {rendered.map(({ line: p, count }, i) => {
                  const lv = p.level;
                  const st = streamOf(p, appFilter);
                  // Accesos/rechazados traen resumen propio; sistema/app
                  // muestran msg + bits. Sin índice y sin tag: el nivel
                  // colorea toda la línea.
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
                      key={i}
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
                })}
                {lines.vis.length === 0 && (
                  <p className="px-3 py-4 text-console-muted">{t("ops.logs.empty")}</p>
                )}
              </div>
            </div>
          </div>
        )}

        {tab === "build" && (
          <div className="space-y-3 p-2">
            <div className="flex flex-wrap items-center gap-2 border border-border bg-card/35 px-2.5 py-2">
              {latest ? (
                <>
                  <Dot status={latest.status} pulse={latest.status === "running"} />
                  <Avatar name={latest.app} />
                  <span className="tnum font-mono text-[10px]">
                    #{latest.id} {latest.app} {latest.rev.slice(0, 8)} <span className="text-muted-foreground">({latest.status})</span>
                  </span>
                  <span className="flex-1" />
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      setBuild(t("common.loading"));
                      deploymentLog(latest.id)
                        .then(setBuild)
                        .catch((e) => setBuild(e instanceof Error ? e.message : t("ops.logs.noLog")));
                    }}
                  >
                    {t("ops.logs.loadBuild")}
                  </Button>
                </>
              ) : (
                <span className="font-mono text-[10px] text-muted-foreground">
                  {appFilter
                    ? t("ops.logs.noDeploymentsApp", { app: appFilter })
                    : t("ops.logs.noDeployments")}
                </span>
              )}
            </div>
            <div className="border border-border">
              <div className="flex items-center gap-2 border-b border-console-border bg-console-raised px-3 py-1.5">
                <span className="micro text-console-muted">{t("ops.logs.buildLog")}</span>
                <span className="flex-1" />
                <span className="tnum font-mono text-[9px] text-console-muted">
                  {latest ? `#${latest.id}` : "—"}
                </span>
              </div>
              <pre className="console h-[58vh] overflow-auto p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap">
                {build}
              </pre>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
