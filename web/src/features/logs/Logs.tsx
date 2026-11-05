import { useEffect, useMemo, useRef, useState } from "react";
import { Download, Pause, Play, Search } from "lucide-react";
import { AppStatus, Deployment } from "../../shared/lib/types";
import { deploymentLog } from "../deployments/api";
import { streamLog } from "../../shared/lib/http";
import { fetchLog } from "./api";
import { Bars } from "../../shared/components/feedback";
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

type Level = "info" | "warning" | "error";

function levelOf(line: string): Level {
  if (/\b(ERROR|FATAL|error=|level=error)/.test(line)) return "error";
  if (/\b(WARN|WARNING|warn=|level=warn)/.test(line)) return "warning";
  return "info";
}

const LEVEL_TONE: Record<Level, string> = {
  info: "bg-muted-foreground/45",
  warning: "bg-warn",
  error: "bg-bad",
};

const LEVEL_TEXT: Record<Level, string> = {
  info: "text-muted-foreground",
  warning: "text-warn",
  error: "text-bad",
};

const LEVEL_KEY: Record<Level, string> = {
  info: "ops.logs.level.info",
  warning: "ops.logs.level.warning",
  error: "ops.logs.level.error",
};

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
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={on}
      className={`inline-flex h-7 items-center gap-1.5 border px-2 font-mono text-[9px] tracking-[0.08em] uppercase transition-colors ${
        on ? "border-input bg-accent/60 text-foreground" : "border-border bg-transparent text-muted-foreground/60 hover:text-foreground"
      }`}
    >
      <span aria-hidden className={`h-1.5 w-1.5 ${on ? LEVEL_TONE[level] : "bg-muted-foreground/25"}`} />
      {t(LEVEL_KEY[level])}
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
    const counts: Record<Level, number> = { info: 0, warning: 0, error: 0 };
    for (const l of all) counts[levelOf(l)]++;
    const rx = q.trim() === "" ? null : new RegExp(q.trim().replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "i");
    const vis = all.filter((l) => levels[levelOf(l)] && (!rx || rx.test(l)));
    const buckets = new Array<number>(24).fill(0);
    let stamped = 0;
    const nowH = Date.now();
    for (const l of all) {
      const m = /time=(\d{4}-\d{2}-\d{2})T(\d{2})/.exec(l);
      if (!m) continue;
      const t = new Date(`${m[1]}T${m[2]}:00:00Z`).getTime();
      if (Number.isNaN(t)) continue;
      const h = Math.floor((nowH - t) / 3600000);
      if (h >= 0 && h < 24) {
        buckets[23 - h]++;
        stamped++;
      }
    }
    return { vis, counts, buckets, stamped };
  }, [log, q, levels]);

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
                aria-live={live ? "polite" : "off"}
                aria-label={t("ops.logs.logAria")}
                className="h-[58vh] overflow-auto bg-ink/45 font-mono text-[11px] leading-[1.6]"
              >
                {lines.vis.map((l, i) => {
                  const lv = levelOf(l);
                  return (
                    <div key={i} className="flex items-baseline gap-2 px-3 hover:bg-accent/25">
                      <span className="tnum w-8 shrink-0 text-right text-[9px] text-muted-foreground/30 select-none">
                        {String(i + 1).padStart(3, "0")}
                      </span>
                      <span aria-label={t("ops.logs.lineLevel", { level: t(LEVEL_KEY[lv]) })} className={`h-1.5 w-1.5 shrink-0 translate-y-[-1px] ${LEVEL_TONE[lv]}`} />
                      <span className={`min-w-0 flex-1 break-words whitespace-pre-wrap ${lv === "error" ? "text-bad/90" : lv === "warning" ? "text-warn/85" : "text-foreground/80"}`}>
                        {l}
                      </span>
                    </div>
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
