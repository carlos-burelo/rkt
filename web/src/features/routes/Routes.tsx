import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";
import { ArrowRight, ExternalLink, Plus } from "lucide-react";
import { Route, Status } from "../../shared/lib/types";
import { addRoute, delRoute, putRoute, domainDetail, preflightDomain, listRoutes } from "./api";
import { useProxyStatus } from "../proxy/hooks";
import { buildLinks, GraphLink } from "../proxy/graph";
import { Proxy } from "../proxy/Proxy";
import { DomainDrawer } from "./DomainDrawer";
import { FunnelMap } from "./FunnelMap";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { CopyButton } from "../../shared/components/feedback";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { Avatar } from "../../shared/components/ui";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { LocalZones } from "./LocalZones";
import { useFleet } from "../overview/hooks";
import { publicUrl, publicHost } from "../../shared/lib/url";
import { useI18n } from "../../shared/i18n";

function TLSBadge({ host, onOpen }: { host: string; onOpen: () => void }) {
  const { t } = useI18n();
  const [txt, setTxt] = useState("…");
  const [state, setState] = useState<"off" | "ok" | "warn" | "bad">("off");
  useEffect(() => {
    domainDetail(host)
      .then((d) => {
        if (!d.tls.managed) {
          setState("off");
          setTxt("");
          return;
        }
        setTxt(d.tls.expires || t("common.yes"));
        const days = d.tls.expires ? Math.ceil((new Date(d.tls.expires).getTime() - Date.now()) / 86400000) : 999;
        setState(Number.isNaN(days) ? "ok" : days <= 0 ? "bad" : days <= 14 ? "warn" : "ok");
      })
      .catch(() => {
        setState("off");
        setTxt("?");
      });
  }, [host, t]);
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

function RouteRow({ r, onChanged, status, onOpen }: { r: Route; onChanged: () => void; status: Status | null; onOpen: () => void }) {
  const { t } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const [target, setTarget] = useState(r.target);
  const [edit, setEdit] = useState(false);
  useEffect(() => {
    setTarget(r.target);
  }, [r.target]);
  const save = async () => {
    await putRoute(r.hosts[0], target.trim());
    onChanged();
  };
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
            onSubmit={save}
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
            onConfirm={() => {
              delRoute(r.hosts[0]).then(onChanged).catch(fail);
            }}
          />
        </div>
      )}
    </div>
  );
}

export function Routes() {
  const { t, tn } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const { status } = useFleet();
  const { rows: proxyRows, reload: reloadProxy } = useProxyStatus();
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") === "diagnostico" ? "diagnostico" : "rutas";
  const [routes, setRoutes] = useState<Route[]>([]);
  const [hosts, setHosts] = useState("");
  const [target, setTarget] = useState("");
  const [mapLinks, setMapLinks] = useState<GraphLink[]>([]);
  const [addOpen, setAddOpen] = useState(false);
  const [dnsMsg, setDnsMsg] = useState("");
  const [selectedHost, setSelectedHost] = useState<string | null>(null);

  async function load() {
    try {
      setRoutes(await listRoutes());
    } catch (e) {
      fail(e);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (proxyRows) {
      setMapLinks(buildLinks(proxyRows));
      return;
    }
    setMapLinks(
      routes.flatMap((r) =>
        r.hosts.map((h) => ({
          host: h,
          target: r.target,
          short: r.target.replace(/^https?:\/\//, "").replace(/^127\.0\.0\.1/, "local"),
          ok: true,
          tls: "off",
          stale: true,
        })),
      ),
    );
  }, [proxyRows, routes]);

  const hostCount = routes.reduce((n, r) => n + r.hosts.length, 0);

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("nav.routes")}
        description={t("serve.routes.desc")}
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {tn("serve.routes.count", routes.length)} · {tn("serve.hosts.count", hostCount)}
          </span>
        }
        actions={
          canWrite && tab === "rutas" ? (
            <FormDialog
              open={addOpen}
              onOpenChange={setAddOpen}
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
                await addRoute(
                  hosts.split(",").map((s) => s.trim()).filter(Boolean),
                  target.trim(),
                );
                setHosts("");
                setTarget("");
                await load();
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
          ) : undefined
        }
      />
      <Tabs value={tab} onValueChange={(v) => setParams(v === "diagnostico" ? { tab: "diagnostico" } : {}, { replace: true })}>
        <TabsList className="w-max">
          <TabsTrigger value="rutas">{t("serve.routes.tab")}</TabsTrigger>
          <TabsTrigger value="diagnostico">{t("serve.routes.diagnostics")}</TabsTrigger>
        </TabsList>
      </Tabs>

      {tab === "diagnostico" && <Proxy embedded />}

      {tab === "rutas" && (
        <>
          <LocalZones onChanged={load} />

          <FunnelMap links={mapLinks} onAdd={() => setAddOpen(true)} />

          <div className="border border-border" role="table" aria-label={t("serve.routes.tableAria")}>
            <div
              className="hidden grid-cols-[1.5rem_minmax(0,1.4fr)_auto_1.5rem_minmax(0,1.4fr)_auto] items-center gap-2 border-b border-border bg-secondary/25 px-3 py-1.5 md:grid"
              aria-hidden
            >
              <span className="micro">{t("serve.field.stateShort")}</span>
              <span className="micro">{t("serve.field.host")}</span>
              <span className="micro">{t("serve.field.tls")}</span>
              <span />
              <span className="micro">{t("serve.field.target")}</span>
              <span />
            </div>
            {routes.map((r) => (
              <RouteRow
                key={r.hosts.join(",") + r.target}
                r={r}
                onChanged={() => {
                  load();
                  reloadProxy();
                }}
                status={status}
                onOpen={() => setSelectedHost(r.hosts[0])}
              />
            ))}
            {routes.length === 0 && (
              <EmptyState title={t("serve.routes.emptyTitle")} hint={t("serve.routes.emptyHint")} />
            )}
          </div>
        </>
      )}
      <DomainDrawer host={selectedHost} onClose={() => setSelectedHost(null)} />
    </div>
  );
}
