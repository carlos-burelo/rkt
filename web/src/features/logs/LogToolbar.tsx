import { Download, Pause, Play, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useUI } from "../../app/ui";
import { useFleet } from "../overview/hooks";
import { useI18n } from "../../shared/i18n";
import { useLogs } from "./logs.store";
import type { LineBuckets } from "./useLogLines";
import { LevelToggle, StreamToggle } from "./logparts";

// Barra de la consola: tabs + app + buscar + niveles + corrientes +
// vivo/pausar/recargar/descargar. Lee y escribe el store.
export function LogToolbar({ lines }: { lines: LineBuckets }) {
  const { t } = useI18n();
  const { status } = useFleet();
  const apps = status?.apps ?? [];
  const appFilter = useUI((s) => s.appFilter);
  const setAppFilter = useUI((s) => s.setAppFilter);
  const tab = useLogs((s) => s.tab);
  const q = useLogs((s) => s.q);
  const live = useLogs((s) => s.live);
  const levels = useLogs((s) => s.levels);
  const stream = useLogs((s) => s.stream);
  const st = useLogs.getState();
  return (
    <div className="flex flex-wrap items-center gap-2 border-b border-border p-2">
      <Tabs value={tab} onValueChange={(v) => st.setTab(v as "runtime" | "build")}>
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
              onChange={(e) => st.setQ(e.target.value)}
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
                onClick={() => st.toggleLevel(l)}
              />
            ))}
          </div>
          {appFilter === "" && (
            <div className="flex items-center gap-px" role="group" aria-label={t("ops.logs.streamsAria")}>
              <button
                type="button"
                onClick={() => st.setStream("all")}
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
                  onClick={() => st.setStream(stream === s ? "all" : s)}
                />
              ))}
            </div>
          )}
          <span className="flex-1" />
          <Button variant="outline" size="sm" onClick={() => st.setLive(!live)}>
            {live ? <Pause className="h-3 w-3" /> : <Play className="h-3 w-3" />}
            {live ? t("ops.logs.pause") : t("ops.logs.live")}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => st.load()}>{t("ops.logs.reload")}</Button>
          <Button variant="ghost" size="sm" onClick={() => st.download()}>
            <Download className="h-3 w-3" /> {t("ops.logs.download")}
          </Button>
        </>
      )}
    </div>
  );
}
