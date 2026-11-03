import { useEffect, useState } from "react";
import { listRules, addRule, delRule, AlertRule } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader, EmptyState, Section } from "../../shared/components/patterns";
import { Dot, StateRail } from "../../shared/components/ui";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

export function Alerts() {
  const canWrite = useAuth((a) => a.canWrite);
  const fail = useUI((u) => u.fail);
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [app, setApp] = useState("*");
  const [kind, setKind] = useState("failed");
  const [threshold, setThreshold] = useState("1");
  const [windowMin, setWindowMin] = useState("60");

  async function load() {
    try {
      setRules(await listRules());
    } catch (e) {
      fail(e);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const fired = rules.filter((r) => r.last_fired).length;

  return (
    <div className="space-y-4">
      <PageHeader
        title="Alertas"
        description="Reglas evaluadas cada minuto. El disparo queda como evento kind=alert en el manifiesto de Resumen."
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {rules.length} reglas · {fired} disparadas
          </span>
        }
      />
      <div className="border border-border" role="table" aria-label="reglas de alerta">
        <div
          className="hidden grid-cols-[1.5rem_3rem_minmax(0,1fr)_7rem_8rem_9rem_4rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 md:grid"
          aria-hidden
        >
          <span className="micro">est</span>
          <span className="micro">folio</span>
          <span className="micro">regla</span>
          <span className="micro">tipo</span>
          <span className="micro">umbral</span>
          <span className="micro text-right">último disparo</span>
          <span />
        </div>
        {rules.map((r) => (
          <div
            key={r.id}
            className="relative grid grid-cols-[1.5rem_2.5rem_minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 py-2.5 pr-3 pl-3 last:border-0 md:grid-cols-[1.5rem_3rem_minmax(0,1fr)_7rem_8rem_9rem_4rem] md:gap-3"
          >
            <StateRail status={r.last_fired ? "degraded" : "stopped"} />
            <Dot status={r.last_fired ? "degraded" : "stopped"} />
            <span className="tnum font-mono text-[10px] text-muted-foreground">#{r.id}</span>
            <div className="min-w-0">
              <p className="truncate font-mono text-[11px]">{r.app}</p>
              <p className="tnum mt-0.5 font-mono text-[9px] text-muted-foreground md:hidden">
                {r.kind} · ≥{r.threshold}/{r.window_min}min ·{" "}
                {r.last_fired ? r.last_fired.slice(0, 16).replace("T", " ") : "sin disparos"}
              </p>
            </div>
            <span className="hidden font-mono text-[10px] text-muted-foreground md:block">{r.kind}</span>
            <span className="tnum hidden font-mono text-[10px] text-muted-foreground md:block">
              ≥{r.threshold}/{r.window_min}min
            </span>
            {r.last_fired ? (
              <span className="tnum hidden text-right font-mono text-[10px] text-warn md:block">
                {r.last_fired.slice(0, 16).replace("T", " ")}
              </span>
            ) : (
              <span className="hidden text-right font-mono text-[10px] text-muted-foreground md:block">sin disparos</span>
            )}
            <div className="flex justify-end">
              {canWrite && (
                <Button
                  variant="ghost"
                  size="xs"
                  className="text-muted-foreground hover:text-bad"
                  onClick={() => delRule(r.id).then(load).catch(fail)}
                >
                  borrar
                </Button>
              )}
            </div>
          </div>
        ))}
        {rules.length === 0 && (
          <EmptyState title="Sin reglas" hint="failed = caídas por app · error_rate = 5xx global (0.1 = 10%)." />
        )}
      </div>

      {canWrite && (
        <Section title="Nueva regla" description="se evalúa cada minuto · el disparo queda como evento kind=alert">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:flex-wrap">
            <label className="field-label w-32 shrink-0">
              app
              <Input value={app} onChange={(e) => setApp(e.target.value)} placeholder="app o *" className="h-7 font-mono text-[11px]" />
            </label>
            <label className="field-label w-36 shrink-0">
              tipo
              <Select value={kind} onValueChange={(v: string | null) => setKind(v ?? "failed")}>
                <SelectTrigger className="h-7 font-mono text-[11px]">
                  <span>{kind}</span>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="failed">failed</SelectItem>
                  <SelectItem value="error_rate">error_rate</SelectItem>
                </SelectContent>
              </Select>
            </label>
            <label className="field-label w-32 shrink-0">
              umbral
              <Input
                value={threshold}
                onChange={(e) => setThreshold(e.target.value)}
                placeholder={kind === "failed" ? "fallos ≥" : "tasa ≥ (0.1)"}
                className="tnum h-7 font-mono text-[11px]"
              />
            </label>
            <label className="field-label w-28 shrink-0">
              ventana
              <Input
                value={windowMin}
                onChange={(e) => setWindowMin(e.target.value)}
                placeholder="min"
                className="tnum h-7 font-mono text-[11px]"
              />
            </label>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                addRule(app.trim() || "*", kind, parseFloat(threshold), parseInt(windowMin, 10))
                  .then(load)
                  .catch(fail);
              }}
            >
              crear
            </Button>
          </div>
        </Section>
      )}
    </div>
  );
}
