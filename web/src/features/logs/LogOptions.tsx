import { Checkbox } from "@/components/ui/checkbox";
import { Bars } from "../../shared/components/feedback";
import { Section } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import { useLogs } from "./logs.store";
import type { LineBuckets } from "./useLogLines";

// Opciones de vista (seguir/compactar/ajustar/hora/contexto) + actividad.
export function LogOptions({ lines, total }: { lines: LineBuckets; total: number }) {
  const { t, tn } = useI18n();
  const st = useLogs.getState();
  const follow = useLogs((s) => s.follow);
  const compact = useLogs((s) => s.compact);
  const wrap = useLogs((s) => s.wrap);
  const absolute = useLogs((s) => s.absolute);
  const context = useLogs((s) => s.context);
  const q = useLogs((s) => s.q);
  return (
    <div className="space-y-3 p-2">
      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
          <Checkbox checked={follow} onCheckedChange={(v) => st.setFollow(v === true)} />
          {t("ops.logs.follow")}
        </label>
        <label className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
          <Checkbox checked={compact} onCheckedChange={(v) => st.setCompact(v === true)} />
          {t("ops.logs.compact")}
        </label>
        <label className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
          <Checkbox checked={wrap} onCheckedChange={(v) => st.setWrap(v === true)} />
          {t("ops.logs.wrap")}
        </label>
        <label className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
          <Checkbox checked={absolute} onCheckedChange={(v) => st.setAbsolute(v === true)} />
          {t("ops.logs.absolute")}
        </label>
        {q.trim() !== "" && (
          <label className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
            <Checkbox checked={context} onCheckedChange={(v) => st.setContext(v === true)} />
            {t("ops.logs.context")}
          </label>
        )}
        <span className="tnum font-mono text-[9px] text-muted-foreground">
          {tn("ops.logs.visibleLines", total, { vis: lines.vis.length })}
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
    </div>
  );
}
