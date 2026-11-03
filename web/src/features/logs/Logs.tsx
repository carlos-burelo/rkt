import { useEffect, useMemo, useRef, useState } from "react";
import { Pause, Play } from "lucide-react";
import { AppStatus, Deployment } from "../../shared/lib/types";
import { deploymentLog } from "../deployments/api";
import { fetchLog } from "./api";
import { Bars } from "../../shared/components/feedback";
import { Dot, Avatar } from "../../shared/components/ui";
import { PageHeader, Section } from "../../shared/components/patterns";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select, SelectValue, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";

type Level = "info" | "warning" | "error";

function levelOf(line: string): Level {
  if (/\b(ERROR|FATAL|error=|level=error)/.test(line)) return "error";
  if (/\b(WARN|WARNING|warn=|level=warn)/.test(line)) return "warning";
  return "info";
}

export function Logs() {
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
  const [build, setBuild] = useState("elige un deployment en Deployments");
  const logRef = useRef<HTMLPreElement>(null);
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
    const t = setInterval(() => {
      fetchLog(appFilter).then(setLog).catch(() => {});
    }, 2000);
    return () => clearInterval(t);
  }, [live, tab, appFilter, online]);

  useEffect(() => {
    if (follow && logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [log, follow]);

  const lines = useMemo(() => {
    const all = log === "" ? [] : log.split("\n");
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

  return (
    <div className="space-y-3">
      <PageHeader
        title="Logs"
        description="Runtime por app u operativo. Live refresca cada 2s; Build muestra el último deploy."
        actions={
          <span className="tnum inline-flex items-center gap-2 font-mono text-[11px] text-muted-foreground">
            <Dot status={tab === "runtime" && live ? "running" : "stopped"} />
            {tab === "runtime" ? (live ? "live · 2s" : "pausado") : "build"}
            {appFilter ? ` · ${appFilter}` : " · operativo"}
          </span>
        }
      />
      <div className="flex flex-wrap items-center gap-2">
        <Tabs value={tab} onValueChange={(v) => setTab(v as "runtime" | "build")}>
          <TabsList>
            <TabsTrigger value="runtime">Runtime</TabsTrigger>
            <TabsTrigger value="build">Build</TabsTrigger>
          </TabsList>
        </Tabs>
        {tab === "runtime" && (
          <>
            <Select value={appFilter || "__all"} onValueChange={(v: string | null) => setAppFilter(v === "__all" ? "" : (v ?? ""))}>
              <SelectTrigger className="w-52 font-mono text-xs">
                <SelectValue placeholder="app" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all">operativo (rkt.log)</SelectItem>
                {apps.map((a) => (
                  <SelectItem key={a.name} value={a.name}>
                    {a.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Buscar…"
              className="h-8 w-48 text-xs"
            />
            {(["info", "warning", "error"] as const).map((l) => (
              <label key={l} className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Checkbox
                  checked={levels[l]}
                  onCheckedChange={() => setLevels({ ...levels, [l]: !levels[l] })}
                />
                {l} <span className="tnum font-mono text-muted-foreground">{lines.counts[l]}</span>
              </label>
            ))}
            <Button
              variant={live ? "default" : "outline"}
              size="sm"
              onClick={() => setLive(!live)}
            >
              {live ? <><Pause className="h-3.5 w-3.5" /> Live</> : <><Play className="h-3.5 w-3.5" /> Live</>}
            </Button>
            <Button variant="outline" size="sm" onClick={load}>
              recargar
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                const blob = new Blob([log], { type: "text/plain" });
                const a = document.createElement("a");
                a.href = URL.createObjectURL(blob);
                a.download = `${appFilter || "rkt"}.log`;
                a.click();
                URL.revokeObjectURL(a.href);
              }}
            >
              descargar
            </Button>
          </>
        )}
      </div>
      {tab === "runtime" && (
        <>
          <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Checkbox checked={follow} onCheckedChange={(v) => setFollow(v === true)} />
            seguir
          </label>
          {lines.stamped > 0 && (
            <Section title="24h" description={`${lines.stamped} líneas con hora · access.log local`}>
              <Bars data={lines.buckets} height={48} />
            </Section>
          )}
          <div className="overflow-hidden rounded-lg border">
            <div className="tnum flex items-center gap-3 border-b bg-muted/40 px-4 py-1.5 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
              <span aria-hidden className="manifest-strip h-4 w-1 rounded-full opacity-70" />
              <span>consola</span>
              <span className="flex-1" />
              <span>{lines.vis.length} líneas</span>
            </div>
            <pre ref={logRef} className="h-[60vh] overflow-auto bg-card/40 p-3 font-mono text-xs leading-relaxed text-foreground">
            {lines.vis.map((l, i) => {
              const lv = levelOf(l);
              return (
                <div key={i} className="flex items-baseline gap-2">
                  <span
                    aria-label={`nivel ${lv}`}
                    className={`inline-block h-1.5 w-1.5 shrink-0 translate-y-[-1px] rounded-full ${
                      lv === "error" ? "bg-red-400" : lv === "warning" ? "bg-amber-400" : "bg-muted-foreground/40"
                    }`}
                  />
                  <span className="min-w-0 flex-1 break-words">{l}</span>
                </div>
              );
            })}
            {lines.vis.length === 0 && "sin líneas"}
            </pre>
          </div>
        </>
      )}
      {tab === "build" && (
        <div className="overflow-hidden rounded-lg border">
          <div className="flex items-center gap-2 border-b bg-muted/40 px-4 py-2 text-xs text-muted-foreground">
            {latest ? (
              <>
                <Dot status={latest.status} />
                <Avatar name={latest.app} />
                <span className="tnum font-mono">
                  #{latest.id} {latest.app} {latest.rev.slice(0, 8)} ({latest.status})
                </span>
                <span className="flex-1" />
                <Button
                  variant="outline" size="sm"
                  onClick={() => {
                    setBuild("cargando…");
                    deploymentLog(latest.id)
                      .then(setBuild)
                      .catch((e) => setBuild(e instanceof Error ? e.message : "sin log"));
                  }}
                >
                  cargar build log
                </Button>
              </>
            ) : (
              <span>Sin deployments{appFilter ? ` para ${appFilter}` : ""}.</span>
            )}
          </div>
          <pre className="h-[60vh] overflow-auto bg-card/40 p-3 font-mono text-xs leading-relaxed text-foreground">
            {build}
          </pre>
        </div>
      )}
    </div>
  );
}
