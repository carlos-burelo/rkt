import { useEffect, useMemo, useRef, useState } from "react";
import { Download, Pause, Play, Search } from "lucide-react";import { AppStatus, Deployment } from "../../shared/lib/types";
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
import { groupParsed, parseLogLine, type Level } from "./format";

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

const LEVEL_BADGE: Record<Level, string> = {
  info: "border-info/40 bg-info/10 text-info",
  warning: "border-warn/45 bg-warn/10 text-warn",
  error: "border-bad/45 bg-bad/10 text-bad",
};

const FIELD_SHORT: Record<string, string> = {
  timestamp: "ts",
  time: "t",
  duration: "dur",
  duration_ms: "dur",
  latency: "lat",
  latency_ms: "lat",
  status: "st",
  status_code: "st",
  method: "m",
  request_id: "req",
  trace_id: "trace",
  user_agent: "ua",
  remote_addr: "ip",
  service: "svc",
  proto: "pr",
  bytes: "b",
  error: "err",
  query: "q",
};

function shortField(key: string): string {
  return FIELD_SHORT[key] ?? key;
}

function fieldTone(key: string, value: string): string {
  const short = shortField(key);
  if (short === "st") {
    const n = Number(value);
    if (n >= 500) return "text-bad";
    if (n >= 400) return "text-warn";
    if (n >= 200) return "text-ok";
  }
  if (short === "m") {
    const v = value.toUpperCase();
    if (v === "DELETE") return "text-bad";
    if (v === "POST" || v === "PUT" || v === "PATCH") return "text-signal";
    return "text-info";
  }
  if (short === "dur" || short === "lat") return "text-info";
  if (short === "err") return "text-bad";
  if (short === "level" || short === "lvl") return "text-foreground/70";
  return "text-foreground/85";
}

function splitScope(msg: string): { scope: string; text: string } {
  const m = /^([A-Za-z0-9_.-]+):\s*(.*)$/.exec(msg);
  return m ? { scope: m[1], text: m[2] } : { scope: "", text: msg };
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
      <mark key={`${i}-${m[0]}`} className="bg-warn/25 text-warn">
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
    for (const p of parsed) counts[p.level]++;
    const rx = q.trim() === "" ? null : new RegExp(q.trim().replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "i");
    const vis = parsed.filter((p) => levels[p.level] && (!rx || rx.test(p.raw)));
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
    return { vis, parsed, counts, buckets, stamped };
  }, [log, q, levels]);

  const rendered = useMemo(() => {
    let list = lines.vis;
    if (context && q.trim() !== "") {
      const rx = new RegExp(q.trim().replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "i");
      const keep = new Set<number>();
      lines.parsed.forEach((p, i) => {
        if (!levels[p.level] || !rx.test(p.raw)) return;
        for (let j = Math.max(0, i - 2); j <= Math.min(lines.parsed.length - 1, i + 2); j++) keep.add(j);
      });
      list = lines.parsed.filter((p, i) => keep.has(i) && levels[p.level]);
    }
    return compact ? groupParsed(list) : list.map((line) => ({ line, count: 1 }));
  }, [compact, context, lines, q, levels]);

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
              <div className="flex items-center gap-2 border-b border-border bg-secondary/25 px-3 py-1.5">
                <span aria-hidden className="h-1 w-1 bg-ok animate-status-pulse" />
                <span className="micro">{t("ops.logs.console")}</span>
                <span className="flex-1" />
                <span className="tnum font-mono text-[9px] text-muted-foreground">
                  {tn("ops.logs.consoleLines", lines.vis.length, { app: appFilter || "rkt.log" })}
                </span>
              </div>
              <div
                ref={logRef}
                role="log"
                tabIndex={0}
                aria-live={live ? "polite" : "off"}
                aria-label={t("ops.logs.logAria")}
                className="h-[58vh] overflow-auto bg-ink/45 font-mono text-[11px] leading-[1.6]"
              >
                {rendered.map(({ line: p, count }, i) => {
                  const lv = p.level;
                  const { scope, text } = splitScope(p.msg || p.raw);
                  return (
                    <div
                      key={i}
                      title={p.raw}
                      className="flex items-center gap-2 px-3 py-0.5 hover:bg-accent/25"
                    >
                      <span className="tnum w-8 shrink-0 self-start text-right text-[9px] text-muted-foreground/30 select-none">
                        {String(i + 1).padStart(3, "0")}
                      </span>
                      <span className="tnum w-14 shrink-0 self-start text-[9px] text-muted-foreground/55">{absolute ? p.time : ago(p.timestamp)}</span>
                      <span
                        aria-label={t("ops.logs.lineLevel", { level: t(LEVEL_KEY[lv]) })}
                        title={t(LEVEL_KEY[lv])}
                        className={`w-8 shrink-0 self-start border px-1 text-center font-mono text-[9px] font-semibold tracking-[0.04em] ${LEVEL_BADGE[lv]}`}
                      >
                        {LEVEL_SHORT[lv]}
                      </span>
                      {compact && p.structured && scope && (
                        <span
                          title={scope}
                          className="hidden shrink-0 self-start border border-border bg-card/40 px-1 font-mono text-[9px] tracking-[0.04em] text-muted-foreground uppercase sm:inline"
                        >
                          {scope}
                        </span>
                      )}
                      <span
                        className={`min-w-0 flex-1 font-mono text-[11px] ${
                          wrap ? "break-words whitespace-pre-wrap" : "truncate"
                        } ${
                          lv === "error" ? "text-bad/90" : lv === "warning" ? "text-warn/85" : "text-foreground/80"
                        }`}
                      >
                        {compact ? (p.structured ? <Highlight text={text} q={q} /> : <Highlight text={p.raw} q={q} />) : <Highlight text={p.raw} q={q} />}
                      </span>
                      {compact && p.structured && p.fields.length > 0 && (
                        <span className="hidden max-w-[52%] shrink-0 items-center gap-1 overflow-hidden sm:flex">
                          {p.fields.slice(0, 4).map((f, idx) => (
                            <span
                              key={`${f.key}-${idx}`}
                              className="whitespace-nowrap border border-border bg-secondary/35 px-1 font-mono text-[9px]"
                              title={`${f.key}=${f.value}`}
                            >
                              <span className="text-muted-foreground">{shortField(f.key)}=</span>
                              <span className={fieldTone(f.key, f.value)}>
                                {f.value.length > 32 ? `${f.value.slice(0, 32)}…` : f.value}
                              </span>
                            </span>
                          ))}
                          {p.fields.length > 4 && (
                            <span
                              title={p.fields.slice(4).map((f) => `${f.key}=${f.value}`).join(" ")}
                              className="border border-border/60 px-1 font-mono text-[9px] text-muted-foreground"
                            >
                              +{p.fields.length - 4}
                            </span>
                          )}
                        </span>
                      )}
                      {count > 1 && (
                        <span className="tnum shrink-0 font-mono text-[9px] text-muted-foreground/60">×{count}</span>
                      )}
                      <CopyButton text={p.raw} />                    </div>
                  );
                })}
                {lines.vis.length === 0 && (
                  <p className="px-3 py-4 text-muted-foreground">{t("ops.logs.empty")}</p>
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
              <div className="flex items-center gap-2 border-b border-border bg-secondary/25 px-3 py-1.5">
                <span className="micro">{t("ops.logs.buildLog")}</span>
                <span className="flex-1" />
                <span className="tnum font-mono text-[9px] text-muted-foreground">
                  {latest ? `#${latest.id}` : "—"}
                </span>
              </div>
              <pre className="h-[58vh] overflow-auto bg-ink/45 p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-foreground/80">
                {build}
              </pre>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
