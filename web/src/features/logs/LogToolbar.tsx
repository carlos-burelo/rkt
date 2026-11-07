import { Download, Pause, Play, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { MultiFilter, SingleFilter } from "../../shared/components/filters";
import { RowMenu } from "../../shared/components/menus";
import { useUI } from "../../app/ui";
import { useFleet } from "../overview/hooks";
import { useI18n } from "../../shared/i18n";
import { useLogs } from "./logs.store";
import type { LineBuckets } from "./useLogLines";

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
          <MultiFilter
            label={t("ops.logs.levelsLabel")}
            items={(["info", "warning", "error"] as const).map((l) => ({
              key: l,
              label: t(`ops.logs.level.${l}`),
              checked: levels[l],
              count: lines.counts[l],
            }))}
            onToggle={(k) => st.toggleLevel(k as keyof typeof levels)}
          />
          {appFilter === "" && (
            <SingleFilter
              value={stream}
              onChange={(v) => st.setStream(v)}
              label={t("ops.logs.streamsLabel")}
              items={[
                { value: "all" as const, label: t("ops.logs.stream.all"), count: lines.counts.info + lines.counts.warning + lines.counts.error },
                ...(["access", "rejected", "system"] as const).map((s) => ({
                  value: s as "access" | "rejected" | "system",
                  label: t(`ops.logs.stream.${s}`),
                  count: lines.streams[s],
                })),
              ]}
            />
          )}
          <span className="flex-1" />
          <Button variant="outline" size="sm" onClick={() => st.setLive(!live)}>
            {live ? <Pause className="h-3 w-3" /> : <Play className="h-3 w-3" />}
            {live ? t("ops.logs.pause") : t("ops.logs.live")}
          </Button>
          <RowMenu label={t("ops.logs.moreActions")}>
            <DropdownMenuItem onClick={() => st.load()}>{t("ops.logs.reload")}</DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => st.download()}
            >
              <Download className="h-3 w-3" /> {t("ops.logs.download")}
            </DropdownMenuItem>
          </RowMenu>
        </>
      )}
    </div>
  );
}
