import { useEffect } from "react";
import { Outlet, useLocation, useNavigate, useParams } from "react-router";
import { ExternalLink } from "lucide-react";
import { Dot } from "../../shared/components/ui";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useFleet } from "../overview/hooks";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

const TABS = [
  { id: "overview", label: "Overview" },
  { id: "deploys", label: "Deployments" },
  { id: "logs", label: "Logs" },
  { id: "analytics", label: "Analytics" },
  { id: "env", label: "Environment" },
  { id: "settings", label: "Settings" },
] as const;

// AppTabs ficha por app con tabs anidadas (el scroll-muro murió).
// Fija appFilter para que Deployments/Logs reutilicen su filtro.
export function AppTabs() {
  const { name = "" } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const { status, metas } = useFleet();
  const act = useUI((u) => u.act);
  const setAppFilter = useUI((u) => u.setAppFilter);
  const { canWrite } = useAuth();
  const active = location.pathname.split("/").pop() ?? "overview";

  useEffect(() => {
    if (name) setAppFilter(name);
  }, [name, setAppFilter]);

  const meta = metas.find((m) => m.name === name);
  const st = status?.apps.find((a) => a.name === name);
  const running = st ? st.running && !st.failed : false;
  const domains = meta?.domains ?? [];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="font-mono text-lg">{name || "(sin app)"}</h2>
        <Dot status={st?.failed ? "failed" : running ? "running" : "stopped"} />
        <span className="flex-1" />
        {domains[0] && (
          <a
            href={`https://${domains[0]}`}
            target="_blank"
            rel="noreferrer"
            className="inline-flex h-8 items-center gap-2 whitespace-nowrap rounded-md border border-input bg-background px-3 text-xs font-medium shadow-xs hover:bg-accent hover:text-accent-foreground"
          >
            <ExternalLink className="h-3.5 w-3.5" /> Visit
          </a>
        )}
        {canWrite && name && (
          <>
            <Button variant="outline" size="sm" onClick={() => act(name, "restart")}>Restart</Button>
            <Button variant="outline" size="sm" onClick={() => act(name, "deploy")}>Redeploy</Button>
            <Button variant="outline" size="sm" onClick={() => act(name, "rollback")}>Rollback</Button>
          </>
        )}
      </div>
      {st && (
        <p className="font-mono text-xs text-muted-foreground">
          {st.failed ? "failed" : st.running ? "running" : "stopped"} · {st.restarts} restarts
          {meta ? ` · ${meta.command}` : ""}
        </p>
      )}
      <Tabs
        value={TABS.some((t) => t.id === active) ? active : "overview"}
        onValueChange={(v) => {
          if (v) navigate(v);
        }}
      >
        <TabsList>
          {TABS.map((t) => (
            <TabsTrigger key={t.id} value={t.id}>
              {t.label}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      <Outlet />
    </div>
  );
}
