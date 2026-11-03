import { useState } from "react";
import { Search } from "lucide-react";
import { Deployment } from "../../shared/lib/types";
import { rel } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { CopyButton } from "../../shared/components/feedback";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";

export type DepStatus = "all" | "ok" | "running" | "failed";

export function Deployments({
  deps,
  appFilter,
  onSelect,
  toolbar = true,
}: {
  deps: Deployment[];
  appFilter: string;
  onSelect: (d: Deployment) => void;
  toolbar?: boolean;
}) {
  const [st, setSt] = useState<DepStatus>("all");
  const [q, setQ] = useState("");
  const f = q.trim().toLowerCase();
  const vis = deps.filter(
    (d) =>
      (appFilter === "" || d.app === appFilter) &&
      (st === "all" || d.status === st) &&
      (f === "" || d.app.toLowerCase().includes(f) || d.rev.toLowerCase().includes(f)),
  );
  return (
    <div className="space-y-3">
      {toolbar && (
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-52 flex-1">
            <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Search deployments…"
              className="pl-8"
            />
          </div>
          <div className="flex gap-1.5">
            {(["all", "ok", "running", "failed"] as const).map((s) => (
              <Badge
                key={s}
                variant={st === s ? "default" : "outline"}
                className="cursor-pointer"
                onClick={() => setSt(s)}
              >
                {s === "all" ? "todos" : s}
              </Badge>
            ))}
          </div>
        </div>
      )}
      <div className="overflow-hidden rounded-lg border">
        {vis.map((d) => (
          <button
            key={d.id}
            onClick={() => onSelect(d)}
            className="flex w-full items-center gap-3 border-b bg-card/40 px-4 py-3 text-left last:border-0 hover:bg-accent/40"
          >
            <Dot status={d.status} />
            <span className="font-mono">{d.app}</span>
            <span className="font-mono text-xs text-muted-foreground">{d.rev.slice(0, 8)}</span>
            <CopyButton text={d.rev} />
            <span className="flex-1" />
            <span className="text-xs text-muted-foreground">
              {d.status === "running" ? "building…" : rel(d.finished_at || d.started_at)}
            </span>
            <span className="font-mono text-xs text-muted-foreground">#{d.id}</span>
          </button>
        ))}
        {vis.length === 0 && (
          <p className="p-6 text-sm text-muted-foreground">Sin deployments para este filtro.</p>
        )}
      </div>
    </div>
  );
}
