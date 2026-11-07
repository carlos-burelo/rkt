import { Bars } from "../../shared/components/feedback";
import { MultiFilter } from "../../shared/components/filters";
import { Section } from "../../shared/components/patterns";
import { useI18n } from "../../shared/i18n";
import { useLogs } from "./logs.store";
import type { LineBuckets } from "./useLogLines";

// Opciones de vista en un dropdown (seguir/compactar/ajustar/hora/
// contexto) + actividad.
export function LogOptions({ lines, total }: { lines: LineBuckets; total: number }) {
  const { t, tn } = useI18n();
  const st = useLogs.getState();
  const follow = useLogs((s) => s.follow);
  const compact = useLogs((s) => s.compact);
  const wrap = useLogs((s) => s.wrap);
  const absolute = useLogs((s) => s.absolute);
  const context = useLogs((s) => s.context);
  const q = useLogs((s) => s.q);
  const toggle = (k: "follow" | "compact" | "wrap" | "absolute" | "context") => {
    const v = { follow, compact, wrap, absolute, context }[k];
    ({ follow: st.setFollow, compact: st.setCompact, wrap: st.setWrap, absolute: st.setAbsolute, context: st.setContext })[k](!v);
  };
  return (
    <div className="space-y-3 p-2">
      <div className="flex flex-wrap items-center gap-2">
        <MultiFilter
          label={t("ops.logs.viewLabel")}
          items={[
            { key: "follow", label: t("ops.logs.follow"), checked: follow },
            { key: "compact", label: t("ops.logs.compact"), checked: compact },
            { key: "wrap", label: t("ops.logs.wrap"), checked: wrap },
            { key: "absolute", label: t("ops.logs.absolute"), checked: absolute },
            { key: "context", label: t("ops.logs.context"), checked: context, disabled: q.trim() === "" },
          ]}
          onToggle={(k) => toggle(k as "follow" | "compact" | "wrap" | "absolute" | "context")}
        />
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
