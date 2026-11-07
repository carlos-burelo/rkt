import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { CopyButton } from "../../shared/components/feedback";
import { Avatar } from "../../shared/components/ui";
import { ArrowRight, ExternalLink, Plus } from "lucide-react";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import { publicUrl, publicHost } from "../../shared/lib/url";
import type { Route, Status } from "../../shared/lib/types";
import { useRoutes } from "./routes.store";
import { preflightDomain } from "./api";

// Badge TLS por host: lee el detalle cacheado del store (una lectura
// por host aunque haya N filas + drawer abierto).
export function TLSBadge({ host, onOpen }: { host: string; onOpen: () => void }) {
  const { t } = useI18n();
  const detail = useRoutes((s) => (host ? s.details[host] : undefined));

  useEffect(() => {
    useRoutes.getState().detail(host);
  }, [host]);

  let txt = "…";
  let state: "off" | "ok" | "warn" | "bad" = "off";
  const d = detail?.data;
  if (d) {
    if (!d.tls.managed) {
      state = "off";
      txt = "";
    } else {
      txt = d.tls.expires || t("common.yes");
      const days = d.tls.expires ? Math.ceil((new Date(d.tls.expires).getTime() - Date.now()) / 86400000) : 999;
      state = Number.isNaN(days) ? "ok" : days <= 0 ? "bad" : days <= 14 ? "warn" : "ok";
    }
  } else if (detail && detail.error) {
    txt = "?";
  }
  const cls =
    state === "bad"
      ? "border-bad/40 bg-bad/10 text-bad"
      : state === "warn"
        ? "border-warn/40 bg-warn/10 text-warn"
        : state === "ok"
          ? "border-ok/30 bg-ok/10 text-ok"
          : "border-border bg-transparent text-muted-foreground";
  return (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        onOpen();
      }}
      title={t("serve.domain.open")}
      className={`inline-flex h-5 w-fit items-center border px-1.5 font-mono text-[10px] tracking-[0.08em] uppercase hover:border-input ${cls}`}
    >
      {state === "off" ? t("serve.tls.none") : t("serve.tls.value", { version: txt })}
    </button>
  );
}

// Fila de ruta con edición inline (borrador local a la fila).
export function RouteRow({ r, status, onOpen }: { r: Route; status: Status | null; onOpen: () => void }) {
  const { t } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const retarget = useRoutes((s) => s.retarget);
  const remove = useRoutes((s) => s.remove);
  const [target, setTarget] = useState(r.target);
  const [edit, setEdit] = useState(false);
  useEffect(() => {
    setTarget(r.target);
  }, [r.target]);
  return (
    <div className="group relative grid grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 px-3 py-2.5 last:border-0 hover:bg-accent/30 md:grid-cols-[1.5rem_minmax(0,1.4fr)_auto_1.5rem_minmax(0,1.4fr)_auto]">
      <Avatar name={r.hosts[0]} />
      <div className="min-w-0">
        <div className="flex items-center gap-1.5">
          <a
            href={publicUrl(r.hosts[0], status)}
            target="_blank"
            rel="noreferrer"
            title={t("serve.routes.visitTitle", { host: publicHost(r.hosts[0], status) })}
            className="truncate font-mono text-[12px] hover:text-signal hover:underline"
          >
            {r.hosts.join(", ")}
          </a>
          <CopyButton text={r.hosts[0]} />
          <ExternalLink className="h-3 w-3 shrink-0 text-muted-foreground/45" />
        </div>
        <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground md:hidden">→ {r.target}</p>
      </div>
      <TLSBadge host={r.hosts[0]} onOpen={onOpen} />
      <ArrowRight className="hidden h-3 w-3 text-muted-foreground/30 md:block" />
      <span className="tnum col-span-2 min-w-0 truncate font-mono text-[11px] text-muted-foreground md:col-span-1">
        {r.target}
      </span>
      {canWrite && (
        <div className="col-span-3 flex justify-end gap-1 md:col-span-1">
          <FormDialog
            open={edit}
            onOpenChange={setEdit}
            trigger={
              <Button variant="ghost" size="xs">
                {t("common.edit")}
              </Button>
            }
            title={t("serve.routes.editTitle", { host: r.hosts[0] })}
            description={t("serve.routes.editDesc")}
            submitLabel={t("common.save")}
            canSubmit={target.trim() !== "" && target.trim() !== r.target}
            onSubmit={() => retarget(r.hosts[0], target.trim())}
          >
            <Field label={t("serve.field.target")} hint={t("serve.field.targetHint")}>
              <Input
                value={target}
                onChange={(e) => setTarget(e.target.value)}
                aria-label={t("serve.routes.targetAria", { host: r.hosts[0] })}
                className="tnum h-8 font-mono text-[12px]"
                autoFocus
              />
            </Field>
          </FormDialog>
          <ConfirmButton
            title={t("serve.routes.deleteTitle", { host: r.hosts[0] })}
            onConfirm={() => remove(r.hosts[0])}
          />
        </div>
      )}
    </div>
  );
}

// Diálogo de alta con preflight DNS (formulario local).
export function NewRouteDialog({ onAdded }: { onAdded: () => void }) {
  const { t } = useI18n();
  const add = useRoutes((s) => s.add);
  const open = useRoutes((s) => s.addOpen);
  const [hosts, setHosts] = useState("");
  const [target, setTarget] = useState("");
  const [dnsMsg, setDnsMsg] = useState("");
  return (
    <FormDialog
      open={open}
      onOpenChange={(o) => useRoutes.getState().setAddOpen(o)}
      trigger={
        <Button size="sm">
          <Plus className="h-3 w-3" /> {t("serve.routes.new")}
        </Button>
      }
      title={t("serve.routes.newTitle")}
      description={t("serve.routes.newDesc")}
      submitLabel={t("common.create")}
      canSubmit={hosts.trim() !== "" && target.trim() !== ""}
      onSubmit={async () => {
        await add(
          hosts.split(",").map((s) => s.trim()).filter(Boolean),
          target.trim(),
        );
        setHosts("");
        setTarget("");
        onAdded();
      }}
    >
      <Field label={t("serve.field.hosts")} hint={t("serve.field.hostsHint")}>
        <Input
          value={hosts}
          onChange={(e) => setHosts(e.target.value)}
          placeholder={t("serve.routes.hostsPlaceholder")}
          className="h-8 font-mono text-[12px]"
          autoFocus
        />
      </Field>
      <Field label={t("serve.field.target")} hint={t("serve.field.targetHint")}>
        <Input
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          placeholder={t("serve.routes.targetPlaceholder")}
          className="tnum h-8 font-mono text-[12px]"
        />
      </Field>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="xs"
          disabled={hosts.trim() === ""}
          onClick={() => {
            const host = hosts.split(",")[0]?.trim() ?? "";
            setDnsMsg(t("serve.domain.checking"));
            preflightDomain(host)
              .then((r) => setDnsMsg(r.dns_ok ? t("serve.domain.preflightOk", { host }) : t("serve.domain.preflightWarn", { host })))
              .catch(() => setDnsMsg(t("serve.domain.preflightError")));
          }}
        >
          {t("serve.domain.check")}
        </Button>
        {dnsMsg !== "" && <span className="font-mono text-[10px] text-muted-foreground">{dnsMsg}</span>}
      </div>
    </FormDialog>
  );
}
