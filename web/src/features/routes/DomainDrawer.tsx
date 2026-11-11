import { useEffect } from "react";
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { X } from "lucide-react";
import { CopyButton } from "../../shared/components/feedback";
import { StatusText } from "../../shared/components/ui";
import { isWildcard } from "../proxy/graph";
import { useI18n } from "../../shared/i18n";
import { useRoutes } from "./routes.store";

const { Root, Portal, Backdrop, Popup, Title, Close } = DialogPrimitive;

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[7.5rem_minmax(0,1fr)] items-baseline gap-2 border-b border-border/60 px-3 py-2 last:border-0">
      <span className="micro">{label}</span>
      <span className="min-w-0 truncate font-mono text-[11px]">{value || "-"}</span>
    </div>
  );
}

export function DomainDrawer({ host, onClose }: { host: string | null; onClose: () => void }) {
  const { t } = useI18n();
  const entry = useRoutes((s) => (host ? s.details[host] : undefined));
  const data = entry?.data ?? null;
  const err = entry?.error ?? "";

  useEffect(() => {
    if (host) useRoutes.getState().detail(host);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host]);

  const ips = data ? [...data.dns.a, ...data.dns.aaaa] : [];

  return (
    <Root open={host !== null} onOpenChange={(open) => !open && onClose()}>
      <Portal>
        <Backdrop className="fixed inset-0 z-40 bg-black/55 backdrop-blur-[2px]" />
        <Popup className="fixed top-0 right-0 z-50 flex h-full w-full max-w-md flex-col overflow-y-auto border-l border-border bg-background outline-none">
          <header className="flex items-start gap-3 border-b border-border px-4 py-4">
            <div className="min-w-0 flex-1">
              <p className="micro text-signal">{t("serve.domain.title")}</p>
              <Title className="mt-1 block truncate font-mono text-[18px] leading-none font-medium tracking-[-0.03em]">
                {host ?? ""}
              </Title>
              {host && (
                <p className="mt-1.5">
                  <span className="inline-block border border-border px-1.5 py-0.5 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase">
                    {isWildcard(host) ? t("serve.routes.wildcardMatch") : t("serve.routes.exactMatch")}
                  </span>
                </p>
              )}
            </div>
            <Close
              aria-label={t("common.close")}
              className="inline-flex h-7 w-7 items-center justify-center border border-border text-muted-foreground hover:border-input hover:text-foreground"
            >
              <X className="h-3.5 w-3.5" />
            </Close>
          </header>
          <div className="space-y-4 p-4">
            {err !== "" && <p className="border border-bad/35 bg-bad/5 px-3 py-2 font-mono text-[10px] text-bad">{err}</p>}
            {!data && err === "" && <p className="font-mono text-[10px] text-muted-foreground">{t("common.loading")}</p>}
            {data && (
              <>
                <section className="border border-border">
                  <Row label={t("serve.domain.app")} value={data.app || "-"} />
                  <Row
                    label={t("serve.domain.target")}
                    value={
                      <span className="flex items-center gap-1.5">
                        <span className="truncate">{data.target}</span>
                        <CopyButton text={data.target} />
                      </span>
                    }
                  />
                  <Row
                    label={t("serve.field.tls")}
                    value={
                      data.tls.managed ? (
                        <span className="text-ok">{t("serve.tls.valueUpper", { version: data.tls.expires || "auto" })}</span>
                      ) : (
                        <span className="text-muted-foreground">{t("serve.tls.noneUpper")}</span>
                      )
                    }
                  />
                </section>

                <section className="border border-border">
                  <div className="flex items-center gap-2 border-b border-border bg-secondary/25 px-3 py-1.5">
                    <span className="micro">{t("serve.domain.dns")}</span>
                    <span className="flex-1" />
                    <StatusText tone={data.dns_ok ? "ok" : "warn"} className="ml-auto">
                      {data.dns_ok ? t("serve.domain.resolves") : t("serve.domain.noResolve")}
                    </StatusText>
                  </div>
                  {!data.dns_ok && data.expected && (
                    <p className="border-b border-border bg-warn/[0.04] px-3 py-2 font-mono text-[10px] leading-relaxed text-warn">
                      {t("serve.domain.suggestA", { ip: data.expected })}{" "}
                      <CopyButton text={data.expected} />
                    </p>
                  )}
                  <Row label="A" value={data.dns.a.length ? data.dns.a.join(", ") : "-"} />
                  <Row label="AAAA" value={data.dns.aaaa.length ? data.dns.aaaa.join(", ") : "-"} />
                  <Row label="CNAME" value={data.dns.cname || "-"} />
                  <Row label={t("serve.domain.expected")} value={data.expected || t("serve.domain.noExpected")} />
                  {data.dns.error && <Row label={t("serve.domain.error")} value={<span className="text-warn">{data.dns.error}</span>} />}
                  {ips.length > 0 && (
                    <div className="flex justify-end px-3 py-2">
                      <CopyButton text={ips.join("\n")} />
                    </div>
                  )}
                </section>

                <p className="font-mono text-[9px] text-muted-foreground/70">
                  {t("serve.domain.checked", { at: data.checked_at.replace("T", " ").slice(0, 19) })}
                </p>
              </>
            )}
          </div>
        </Popup>
      </Portal>
    </Root>
  );
}
