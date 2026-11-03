import { RefObject, useEffect, useMemo, useState } from "react";
import { AppStatus, Deployment } from "../../shared/lib/types";
import { deploymentLog } from "../deployments/api";
import { fetchLog } from "./api";
import { Bars } from "../../shared/components/feedback";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select, SelectValue, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";

type Level = "info" | "warning" | "error";

function levelOf(line: string): Level {
  if (/\b(ERROR|FATAL|error=|level=error)/.test(line)) return "error";
  if (/\b(WARN|WARNING|warn=|level=warn)/.test(line)) return "warning";
  return "info";
}

export function Logs({
  apps,
  appFilter,
  setAppFilter,
  log,
  setLog,
  follow,
  setFollow,
  refresh,
  logRef,
  deps,
}: {
  apps: AppStatus[];
  appFilter: string;
  setAppFilter: (s: string) => void;
  log: string;
  setLog: (s: string) => void;
  follow: boolean;
  setFollow: (b: boolean) => void;
  refresh: () => void;
  logRef: RefObject<HTMLPreElement | null>;
  deps: Deployment[];
}) {
  const [tab, setTab] = useState<"runtime" | "build">("runtime");
  const [q, setQ] = useState("");
  const [live, setLive] = useState(true);
  const [levels, setLevels] = useState<Record<Level, boolean>>({ info: true, warning: true, error: true });
  const [build, setBuild] = useState("elige un deployment en Deployments");
  const latest = deps.find((d) => appFilter === "" || d.app === appFilter);

  useEffect(() => {
    if (!live || tab !== "runtime") return;
    const t = setInterval(() => {
      fetchLog(appFilter).then(setLog).catch(() => {});
    }, 2000);
    return () => clearInterval(t);
  }, [live, tab, appFilter, setLog]);

  const lines = useMemo(() => {
    const all = log === "" ? [] : log.split("\n");
    const counts: Record<Level, number> = { info: 0, warning: 0, error: 0 };
    for (const l of all) counts[levelOf(l)]++;
    const rx = q.trim() === "" ? null : new RegExp(q.trim().replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "i");
    const vis = all.filter((l) => levels[levelOf(l)] && (!rx || rx.test(l)));
    // histograma 24h desde timestamps time=YYYY-MM-DDTHH: (slog)
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
    <div className="space-y-2">
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
                {l} <span className="font-mono text-zinc-600">{lines.counts[l]}</span>
              </label>
            ))}
            <Button
              variant={live ? "default" : "outline"}
              size="sm"
              onClick={() => setLive(!live)}
            >
              {live ? "❚❚ Live" : "▶ Live"}
            </Button>
            <Button variant="outline" size="sm" onClick={refresh}>
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
            <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-3">
              <Bars data={lines.buckets} height={48} />
            </div>
          )}
          <pre ref={logRef} className="h-[60vh] overflow-auto rounded-lg border border-zinc-800 bg-black p-3 font-mono text-xs leading-relaxed text-zinc-300">
            {lines.vis.join("\n") || "sin líneas"}
          </pre>
        </>
      )}
      {tab === "build" && (
        <BuildTab appFilter={appFilter} latest={latest} build={build} setBuild={setBuild} />
      )}
    </div>
  );
}

function BuildTab({
  appFilter,
  latest,
  build,
  setBuild,
}: {
  appFilter: string;
  latest: Deployment | undefined;
  build: string;
  setBuild: (s: string) => void;
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-xs text-zinc-400">
        {latest ? (
          <>
            <span className="font-mono">
              #{latest.id} {latest.app} {latest.rev.slice(0, 8)} ({latest.status})
            </span>
            <button
              onClick={() => {
                setBuild("cargando…");
                deploymentLog(latest.id)
                  .then(setBuild)
                  .catch((e) => setBuild(e instanceof Error ? e.message : "sin log"));
              }}
              className="rounded-md border border-zinc-800 px-2 py-1 hover:text-zinc-100"
            >
              cargar build log
            </button>
          </>
        ) : (
          <span>Sin deployments{appFilter ? ` para ${appFilter}` : ""}.</span>
        )}
      </div>
      <pre className="h-[60vh] overflow-auto rounded-lg border border-zinc-800 bg-black p-3 font-mono text-xs leading-relaxed text-zinc-300">
        {build}
      </pre>
    </div>
  );
}
