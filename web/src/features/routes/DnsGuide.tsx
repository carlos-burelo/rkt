import { useCallback, useEffect, useState } from "react";
import { CopyButton } from "../../shared/components/feedback";
import { StatusText } from "../../shared/components/ui";
import { useI18n } from "../../shared/i18n";
import { domainGuide, type DomainGuide, type DomainGuideRecord } from "./api";

function RecordRow({ rec }: { rec: DomainGuideRecord }) {
  return (
    <div className="flex items-center gap-2 border-b border-border/60 px-3 py-2 font-mono text-[11px] last:border-0">
      <span className="text-muted-foreground">{rec.type}</span>
      <span>{rec.name}</span>
      <span className="text-muted-foreground">→</span>
      <span className="min-w-0 flex-1 truncate">{rec.value}</span>
      <span className="text-muted-foreground">TTL {rec.ttl}</span>
      <CopyButton text={rec.value} />
    </div>
  );
}

/** Proceso DNS paso a paso: el servidor decide estado y pasos (domains.BuildGuide). */
export function DnsGuide({ host }: { host: string }) {
  const { t } = useI18n();
  const [guide, setGuide] = useState<DomainGuide | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const load = useCallback(() => {
    setBusy(true);
    setErr("");
    domainGuide(host)
      .then(setGuide)
      .catch(() => setErr(t("serve.guide.loadError")))
      .finally(() => setBusy(false));
  }, [host, t]);

  useEffect(() => {
    load();
  }, [load]);

  if (err) return <p className="font-mono text-[10px] text-warn">{err}</p>;
  if (!guide) return null;

  return (
    <section className="border border-border">
      <div className="flex items-center gap-2 border-b border-border bg-secondary/25 px-3 py-1.5">
        <span className="micro">{t("serve.guide.title")}</span>
        <span className="flex-1" />
        <StatusText tone={guide.ready ? "ok" : "warn"}>{guide.status}</StatusText>
      </div>
      <p className="border-b border-border px-3 py-2 text-[12px] leading-relaxed">{guide.summary}</p>
      {guide.record && !guide.ready && (
        <>
          <p className="micro border-b border-border/60 px-3 py-1.5">{t("serve.guide.record")}</p>
          <RecordRow rec={guide.record} />
        </>
      )}
      <ol className="space-y-2 px-3 py-2">
        {guide.steps.map((s) => (
          <li key={s.id} className="flex gap-2 text-[11px]">
            <span aria-hidden className={s.done ? "text-ok" : "text-muted-foreground"}>
              {s.done ? "✓" : "○"}
            </span>
            <span className="min-w-0">
              <b className="text-foreground">{s.title}</b>
              <br />
              <span className="text-muted-foreground">{s.detail}</span>
            </span>
          </li>
        ))}
      </ol>
      {guide.wildcard && !guide.ready && (
        <>
          <p className="micro border-y border-border/60 px-3 py-1.5">{t("serve.guide.wildcard")}</p>
          <RecordRow rec={guide.wildcard} />
        </>
      )}
      <div className="flex justify-end border-t border-border px-3 py-2">
        <button
          type="button"
          onClick={load}
          disabled={busy}
          className="font-mono text-[10px] text-muted-foreground hover:underline disabled:opacity-60"
        >
          {busy ? t("serve.guide.checking") : t("serve.guide.recheck")}
        </button>
      </div>
    </section>
  );
}
