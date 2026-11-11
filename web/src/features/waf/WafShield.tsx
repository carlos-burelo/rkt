import { useEffect, useState } from "react";
import { Plus, RefreshCw, ShieldCheck, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Section, Stat, StatGrid } from "../../shared/components/patterns";
import { useAuth } from "../auth/store";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import { rel } from "../../shared/lib/time";
import { fetchWaf, putWaf, unban, type WafState } from "./api";

const POLL_MS = 15000;

// Escudo: reglas del WAF y últimos bloqueos. Solo operator (el backend
// exige rol y los bloqueos exponen IPs atacantes).
export function WafShield() {
  const { t, tn } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const [waf, setWaf] = useState<WafState | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [newPath, setNewPath] = useState("");
  const [newMethod, setNewMethod] = useState("");

  useEffect(() => {
    let alive = true;
    let timer: ReturnType<typeof setInterval> | null = null;
    const load = () => {
      fetchWaf()
        .then((w) => {
          if (!alive) return;
          setWaf(w);
          setErr("");
        })
        .catch((e) => {
          if (!alive) return;
          setErr(e instanceof Error ? e.message : t("common.error"));
        });
    };
    load();
    timer = setInterval(load, POLL_MS);
    return () => {
      alive = false;
      if (timer) clearInterval(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (!canWrite) return null;

  const apply = async (patch: { deny_paths?: string[]; deny_methods?: string[] }) => {
    setBusy(true);
    try {
      setWaf(await putWaf(patch));
      toast.success(t("serve.waf.saved"));
    } catch (e) {
      fail(e);
    } finally {
      setBusy(false);
    }
  };
  const addPath = () => {
    const p = newPath.trim();
    if (!p || !waf || busy) return;
    if (waf.deny_paths.includes(p)) return;
    setNewPath("");
    void apply({ deny_paths: [...waf.deny_paths, p] });
  };
  const addMethod = () => {
    const m = newMethod.trim().toUpperCase();
    if (!m || !waf || busy) return;
    if (waf.deny_methods.includes(m)) return;
    setNewMethod("");
    void apply({ deny_methods: [...waf.deny_methods, m] });
  };

  return (
    <Section
      title={t("serve.waf.title")}
      description={t("serve.waf.desc")}
      accent={!!err}
      actions={
        <Button variant="ghost" size="xs" onClick={() => {
          fetchWaf().then(setWaf).catch(fail);
        }} title={t("serve.waf.refresh")}>
          <RefreshCw className="h-3 w-3" /> {t("serve.waf.refresh")}
        </Button>
      }
    >
      {err !== "" && <p className="mb-3 font-mono text-[10px] text-bad">{err}</p>}
      {waf === null ? (
        <p className="font-mono text-[10px] text-muted-foreground">{t("common.loading")}</p>
      ) : (
        <div className="space-y-4">
          <StatGrid>
            <Stat value={String(waf.deny_paths.length + waf.deny_methods.length)} label={t("serve.waf.rules")} sub={t("serve.waf.rulesSub")} />
            <Stat value={String(waf.deny_methods.length)} label={t("serve.waf.methods")} sub={waf.deny_methods.join(" · ") || "—"} />
            <Stat
              value={String(waf.blocked.length)}
              label={t("serve.waf.blocked")}
              sub={t("serve.waf.blockedSub")}
              tone={waf.blocked.length > 0 ? "warn" : undefined}
            />
            <Stat value={waf.blocked.length ? rel(waf.blocked[0]!.at) : "—"} label={t("serve.waf.lastHit")} sub={waf.blocked[0] ? `${waf.blocked[0].ip} · ${waf.blocked[0].rule}` : t("serve.waf.noHits")} />
          </StatGrid>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <div className="border border-border">
              <p className="micro border-b border-border bg-secondary/20 px-3 py-1.5">{t("serve.waf.methodsTitle")}</p>
              <div className="flex flex-wrap gap-1.5 px-3 py-2">
                {waf.deny_methods.map((m) => (
                  <span key={m} className="inline-flex items-center gap-1 border border-bad/40 bg-bad/10 px-1.5 py-0.5 font-mono text-[10px] text-bad">
                    {m}
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => void apply({ deny_methods: waf.deny_methods.filter((x) => x !== m) })}
                      className="text-bad/60 hover:text-bad"
                      aria-label={t("serve.waf.removeMethod", { method: m })}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </span>
                ))}
                {waf.deny_methods.length === 0 && (
                  <span className="font-mono text-[10px] text-muted-foreground">{t("serve.waf.noMethods")}</span>
                )}
              </div>
              <div className="flex gap-1 border-t border-border p-2">
                <Input
                  value={newMethod}
                  onChange={(e) => setNewMethod(e.target.value.toUpperCase().replace(/[^A-Z]/g, ""))}
                  onKeyDown={(e) => { if (e.key === "Enter") addMethod(); }}
                  placeholder="TRACE"
                  aria-label={t("serve.waf.methodAria")}
                  className="tnum h-7 font-mono text-[11px]"
                />
                <Button size="xs" variant="outline" disabled={busy || newMethod.trim() === ""} onClick={addMethod}>
                  <Plus className="h-3 w-3" /> {t("common.add")}
                </Button>
              </div>
            </div>

            <div className="border border-border">
              <p className="micro border-b border-border bg-secondary/20 px-3 py-1.5">{t("serve.waf.pathsTitle")}</p>
              <ul className="max-h-44 divide-y divide-border/60 overflow-auto">
                {waf.deny_paths.map((p) => (
                  <li key={p} className="flex items-center gap-2 px-3 py-1.5">
                    <ShieldCheck className="h-3 w-3 shrink-0 text-muted-foreground/60" />
                    <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{p}</span>
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => void apply({ deny_paths: waf.deny_paths.filter((x) => x !== p) })}
                      className="text-muted-foreground/60 hover:text-bad"
                      aria-label={t("serve.waf.removePath", { path: p })}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </li>
                ))}
              </ul>
              <div className="flex gap-1 border-t border-border p-2">
                <Input
                  value={newPath}
                  onChange={(e) => setNewPath(e.target.value)}
                  onKeyDown={(e) => { if (e.key === "Enter") addPath(); }}
                  placeholder="/wp-*"
                  aria-label={t("serve.waf.pathAria")}
                  className="h-7 font-mono text-[11px]"
                />
                <Button size="xs" variant="outline" disabled={busy || newPath.trim() === ""} onClick={addPath}>
                  <Plus className="h-3 w-3" /> {t("common.add")}
                </Button>
              </div>
            </div>
          </div>

          <div className="border border-border">
            <p className="micro border-b border-border bg-secondary/20 px-3 py-1.5">
              {t("serve.waf.bannedTitle")} · <span className="tnum">{tn("serve.waf.banned", waf.banned.length)}</span>
            </p>
            <p className="border-b border-border/60 px-3 py-1.5 font-mono text-[9px] text-muted-foreground">
              {t("serve.waf.policy")}
            </p>
            {waf.banned.length === 0 ? (
              <p className="px-3 py-3 font-mono text-[10px] text-muted-foreground">{t("serve.waf.noBanned")}</p>
            ) : (
              <ul className="max-h-44 divide-y divide-border/60 overflow-auto">
                {waf.banned.map((b) => (
                  <li key={b.ip} className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 px-3 py-1.5 font-mono text-[10px]">
                    <span className="tnum text-foreground/80">{b.ip}</span>
                    <span className="tnum text-muted-foreground">{tn("serve.waf.hits", b.hits)}</span>
                    <span className="tnum text-muted-foreground">{t("serve.waf.until", { at: b.until.replace("T", " ").slice(0, 19) })}</span>
                    <span className="flex-1" />
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() =>
                        unban(b.ip)
                          .then(fetchWaf)
                          .then(setWaf)
                          .catch(fail)
                      }
                      className="border border-border px-1.5 py-0.5 font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:border-input hover:text-foreground"
                    >
                      {t("serve.waf.unban")}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="border border-border">
            <p className="micro border-b border-border bg-secondary/20 px-3 py-1.5">
              {t("serve.waf.feed")} · <span className="tnum">{tn("serve.waf.hits", waf.blocked.length)}</span>
            </p>
            {waf.blocked.length === 0 ? (
              <p className="px-3 py-3 font-mono text-[10px] text-muted-foreground">{t("serve.waf.noHits")}</p>
            ) : (
              <ul className="max-h-56 divide-y divide-border/60 overflow-auto">
                {waf.blocked.map((b, i) => (
                  <li key={`${b.at}-${i}`} className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 px-3 py-1.5 font-mono text-[10px]">
                    <span className="tnum text-muted-foreground">{rel(b.at)}</span>
                    <span className="tnum text-foreground/80">{b.ip}</span>
                    <span className="border border-bad/40 bg-bad/10 px-1 text-[9px] text-bad">{b.method}</span>
                    <span className="min-w-0 flex-1 truncate">
                      {b.host}
                      <span className="text-muted-foreground">{b.path}</span>
                    </span>
                    <span className="tnum text-[9px] text-muted-foreground">{b.rule}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      )}
    </Section>
  );
}
