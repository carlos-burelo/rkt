import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { CopyButton } from "../../shared/components/feedback";
import { RowMenu } from "../../shared/components/menus";
import { Avatar } from "../../shared/components/ui";
import { ArrowRight, ExternalLink, Pencil, Plus, Trash2 } from "lucide-react";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import { publicUrl, publicHost } from "../../shared/lib/url";
import { isWildcard } from "../proxy/graph";
import type { Route, Status } from "../../shared/lib/types";
import { useRoutes } from "./routes.store";
import { preflightDomain } from "./api";

// Estrategia de la ruta: dominio o prefijo de ruta.
export type RouteMode = "host" | "path";

export function routeMode(r: Route): RouteMode {
  return r.path ? "path" : "host";
}

// Dirección visible: app.midominio.com · ∗/app · IP/app.
export function routeAddr(r: Route): string {
  const host = r.hosts.length ? r.hosts.join(", ") : "∗";
  return `${host}${r.path ?? ""}`;
}

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
  const mode = routeMode(r);
  const addr = routeAddr(r);
  const hasHost = r.hosts.length > 0;
  return (
    <div className="group relative grid grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 px-3 py-2.5 last:border-0 hover:bg-accent/30 md:grid-cols-[1.5rem_minmax(0,1.4fr)_auto_1.5rem_minmax(0,1.4fr)_auto]">
      <Avatar name={r.hosts[0] ?? addr} />
      <div className="min-w-0">
        <div className="flex items-center gap-1.5">
          {hasHost ? (
            <a
              href={publicUrl(r.hosts[0], status)}
              target="_blank"
              rel="noreferrer"
              title={t("serve.routes.visitTitle", { host: publicHost(r.hosts[0], status) })}
              className="truncate font-mono text-[12px] hover:text-signal hover:underline"
            >
              {addr}
            </a>
          ) : (
            <span className="truncate font-mono text-[12px]" title={t("serve.routes.anyHostTitle")}>
              {addr}
            </span>
          )}
          <CopyButton text={hasHost ? r.hosts[0] : addr} />
          <ExternalLink className="h-3 w-3 shrink-0 text-muted-foreground/45" />
          <span
            title={t(`serve.routes.mode.${mode}Title`)}
            className="shrink-0 border border-border px-1 font-mono text-[8px] tracking-[0.08em] text-muted-foreground/70 uppercase"
          >
            {t(`serve.routes.mode.${mode}`)}
          </span>
          {mode === "host" && (
            <span
              title={t(isWildcard(r.hosts[0] ?? "") ? "serve.routes.wildcardTitle" : "serve.routes.exactTitle")}
              className="shrink-0 border border-border px-1 font-mono text-[8px] tracking-[0.08em] text-muted-foreground/70 uppercase"
            >
              {isWildcard(r.hosts[0] ?? "") ? t("serve.routes.wildcard") : t("serve.routes.exact")}
            </span>
          )}
        </div>
        <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground md:hidden">→ {r.target}</p>
      </div>
      {hasHost ? (
        <TLSBadge host={r.hosts[0]} onOpen={onOpen} />
      ) : (
        <span className="font-mono text-[10px] text-muted-foreground/50">—</span>
      )}
      <ArrowRight className="hidden h-3 w-3 text-muted-foreground/30 md:block" />
      <span className="tnum col-span-2 min-w-0 truncate font-mono text-[11px] text-muted-foreground md:col-span-1">
        {r.target}
      </span>
      {canWrite && (
        <div className="col-span-3 flex justify-end gap-1 md:col-span-1">
          <RowMenu label={t("serve.routes.rowActions", { host: r.hosts[0] })}>
            <FormDialog
              trigger={<DropdownMenuItem><Pencil className="h-3.5 w-3.5" aria-hidden />{t("common.edit")}</DropdownMenuItem>}
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
              icon={<Trash2 className="h-3.5 w-3.5" aria-hidden />}
            />
          </RowMenu>
        </div>
      )}
    </div>
  );
}

// Diálogo de alta con 2 modos visuales (dominio / ruta),
// ejemplo vivo y preflight DNS solo en modo dominio.
export function NewRouteDialog({ onAdded }: { onAdded: () => void }) {
  const { t } = useI18n();
  const add = useRoutes((s) => s.add);
  const open = useRoutes((s) => s.addOpen);
  const [mode, setMode] = useState<RouteMode>("host");
  const [hosts, setHosts] = useState("");
  const [path, setPath] = useState("/app");
  const [target, setTarget] = useState("");
  const [dnsMsg, setDnsMsg] = useState("");

  const hostList = hosts.split(",").map((s) => s.trim()).filter(Boolean);
  const valid =
    target.trim() !== "" &&
    (mode === "host" ? hostList.length > 0 : path.startsWith("/") && path.length > 1);
  const example =
    mode === "host" ? (hostList[0] ?? "app.midominio.com") : `${hostList[0] || "IP"}${path || "/app"}`;

  const reset = () => {
    setHosts("");
    setPath("/app");
    setTarget("");
    setDnsMsg("");
    setMode("host");
  };

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
      canSubmit={valid}
      onSubmit={async () => {
        await add(hostList, target.trim(), mode === "path" ? path : undefined);
        reset();
        onAdded();
      }}
    >
      <div className="grid grid-cols-2 gap-1.5" role="tablist" aria-label={t("serve.routes.modeLabel")}>
        {(["host", "path"] as RouteMode[]).map((m) => (
          <button
            key={m}
            type="button"
            role="tab"
            aria-selected={mode === m}
            onClick={() => setMode(m)}
            className={`border px-2 py-1.5 text-left ${mode === m ? "border-signal bg-accent/40" : "border-border hover:border-input"}`}
          >
            <span className="block font-mono text-[10px] tracking-[0.08em] uppercase">{t(`serve.routes.mode.${m}`)}</span>
            <span className="mt-0.5 block truncate font-mono text-[9px] text-muted-foreground">{t(`serve.routes.mode.${m}Hint`)}</span>
          </button>
        ))}
      </div>
      <p className="border border-border bg-secondary/20 px-2 py-1.5 font-mono text-[10px]">
        <span className="text-muted-foreground">{t("serve.routes.example")}: </span>
        <span className="tnum">{example}</span>
        <span className="text-muted-foreground"> → </span>
        <span className="tnum">{target || "http://127.0.0.1:____"}</span>
      </p>
      {mode === "host" && (
        <Field label={t("serve.field.hosts")} hint={t("serve.field.hostsHint")}>
          <Input
            value={hosts}
            onChange={(e) => setHosts(e.target.value)}
            placeholder={t("serve.routes.hostsPlaceholder")}
            className="h-8 font-mono text-[12px]"
            autoFocus
          />
        </Field>
      )}
      {mode === "path" && (
        <Field label={t("serve.field.hostsOptional")} hint={t("serve.field.hostsOptionalHint")}>
          <Input
            value={hosts}
            onChange={(e) => setHosts(e.target.value)}
            placeholder="192.0.2.10 (vacío = cualquier host)"
            className="h-8 font-mono text-[12px]"
            autoFocus
          />
        </Field>
      )}
      {mode === "path" && (
        <Field label={t("serve.routes.path")} hint={t("serve.routes.pathHint")}>
          <Input
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="/app"
            className="h-8 font-mono text-[12px]"
          />
        </Field>
      )}
      <Field label={t("serve.field.target")} hint={t("serve.field.targetHint")}>
        <Input
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          placeholder={t("serve.routes.targetPlaceholder")}
          className="tnum h-8 font-mono text-[12px]"
        />
      </Field>
      {mode === "host" && (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="xs"
            disabled={hostList.length === 0}
            onClick={() => {
              const host = hostList[0] ?? "";
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
      )}
    </FormDialog>
  );
}
