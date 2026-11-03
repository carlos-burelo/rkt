import { useState } from "react";
import { useNavigate } from "react-router";
import { Search } from "lucide-react";
import { rel } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";
import { CopyButton } from "../../shared/components/feedback";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { useUI } from "../../app/ui";
import { useFleet } from "../overview/hooks";

export type DepStatus = "all" | "ok" | "running" | "failed";

export function Deployments({ bare = false, status, max }: { bare?: boolean; status?: DepStatus; max?: number }) {
  const { deps } = useFleet();
  const appFilter = useUI((u) => u.appFilter);
  const setSel = useUI((u) => u.setSel);
  const navigate = useNavigate();
  const [st, setSt] = useState<DepStatus>(status ?? "all");
  const [q, setQ] = useState("");
  const f = q.trim().toLowerCase();
  let vis = deps.filter(
    (d) =>
      (appFilter === "" || d.app === appFilter) &&
      (st === "all" || d.status === st) &&
      (f === "" || d.app.toLowerCase().includes(f) || d.rev.toLowerCase().includes(f)),
  );
  if (max !== undefined) vis = vis.slice(0, max);
  return (
    <div className="space-y-3">
      {!bare && (
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
          <div
            key={d.id}
            onClick={() => setSel(d)}
            className="flex w-full cursor-pointer items-center gap-3 border-b bg-card/40 px-4 py-3 text-left last:border-0 hover:bg-accent/40"
          >
            <Dot status={d.status} />
            <div className="min-w-0">
              <p className="font-mono text-sm">
                {d.app} <span className="text-muted-foreground">· #{d.id}</span>
              </p>
              <p className="truncate font-mono text-xs text-muted-foreground">
                {d.rev.slice(0, 8)} · {d.triggered_by ? `por ${d.triggered_by}` : "rev"}
              </p>
            </div>
            <CopyButton text={d.rev} />
            <span className="flex-1" />
            <span className="hidden text-xs text-muted-foreground sm:block">
              {d.status === "running" ? "building…" : rel(d.finished_at || d.started_at)}
            </span>
            <button
              className="rounded px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
              onClick={(e) => {
                e.stopPropagation();
                navigate(`/app/${d.app}`);
              }}
            >
              app →
            </button>
          </div>
        ))}
        {vis.length === 0 && (
          <p className="p-6 text-sm text-muted-foreground">Sin deployments para este filtro.</p>
        )}
      </div>
    </div>
  );
}
