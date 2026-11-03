import { useEffect, useState } from "react";
import { listRules, addRule, delRule, AlertRule } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
import {
  Select, SelectValue, SelectTrigger, SelectContent, SelectItem,
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

  return (
    <div className="space-y-3">
      <PageHeader
        title="Alerts"
        description="Reglas evaluadas cada minuto. El disparo queda como evento kind=alert (ver Overview)."
      />
      <div className="overflow-hidden rounded-lg border">
        {rules.map((r) => (
          <div key={r.id} className="flex items-center gap-3 border-b bg-card/40 px-4 py-2.5 last:border-0">
            <span className="font-mono text-xs">#{r.id}</span>
            <span className="font-mono text-xs">{r.app}</span>
            <Badge variant="secondary" className="font-mono text-[11px]">{r.kind}</Badge>
            <span className="font-mono text-xs text-muted-foreground">≥{r.threshold}/{r.window_min}min</span>
            <span className="flex-1" />
            {r.last_fired ? (
              <span className="font-mono text-[11px] text-amber-300">disparó {r.last_fired.slice(0, 16).replace("T", " ")}</span>
            ) : (
              <span className="font-mono text-[11px] text-muted-foreground">sin disparos</span>
            )}
            {canWrite && (
              <Button variant="ghost" size="sm" className="text-xs text-muted-foreground hover:text-red-300"
                onClick={() => delRule(r.id).then(load).catch(fail)}>
                borrar
              </Button>
            )}
          </div>
        ))}
        {rules.length === 0 && (
          <EmptyState title="Sin reglas" hint="failed = caídas por app · error_rate = 5xx global (0.1 = 10%)." />
        )}
      </div>
      {canWrite && (
        <div className="flex flex-wrap gap-2">
          <Input value={app} onChange={(e) => setApp(e.target.value)} placeholder="app o *"
            className="w-32 font-mono text-xs" />
          <Select value={kind} onValueChange={(v: string | null) => setKind(v ?? "failed")}>
            <SelectTrigger className="w-36 font-mono text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="failed">failed</SelectItem>
              <SelectItem value="error_rate">error_rate</SelectItem>
            </SelectContent>
          </Select>
          <Input value={threshold} onChange={(e) => setThreshold(e.target.value)} placeholder={kind === "failed" ? "fallos ≥" : "tasa ≥ (0.1)"}
            className="w-32 font-mono text-xs" />
          <Input value={windowMin} onChange={(e) => setWindowMin(e.target.value)} placeholder="ventana min"
            className="w-28 font-mono text-xs" />
          <Button
            variant="outline" size="sm"
            onClick={() => {
              addRule(app.trim() || "*", kind, parseFloat(threshold), parseInt(windowMin, 10))
                .then(load)
                .catch(fail);
            }}
          >
            crear
          </Button>
        </div>
      )}
    </div>
  );
}
