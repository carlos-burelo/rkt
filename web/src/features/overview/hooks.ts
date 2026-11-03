import { useEffect, useState } from "react";
import { Status, Deployment, RktEvent, AppMeta } from "../../shared/lib/types";
import { cached } from "../../shared/lib/http";
import { fetchStatus, fetchDeployments, fetchEvents } from "./api";
import { listApps } from "../apps/api";
import { useUI } from "../../app/ui";

export interface Fleet {
  status: Status | null;
  deps: Deployment[];
  evs: RktEvent[];
  metas: AppMeta[];
  loading: boolean;
}

// useFleet flota compartida con poll 5s. Respuestas cacheadas 4s:
// sidebar, página y paleta montadas a la vez = una request.
export function useFleet(): Fleet {
  const fail = useUI((u) => u.fail);
  const online = useUI((u) => u.online);
  const [status, setStatus] = useState<Status | null>(null);
  const [deps, setDeps] = useState<Deployment[]>([]);
  const [evs, setEvs] = useState<RktEvent[]>([]);
  const [metas, setMetas] = useState<AppMeta[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!online) return;
    let stop = false;
    async function load() {
      try {
        const [st, dp, ev, am] = await Promise.all([
          cached("fleet:status", 4000, fetchStatus),
          cached("fleet:deps", 4000, () => fetchDeployments()),
          cached("fleet:evs", 4000, () => fetchEvents()),
          cached("fleet:apps", 4000, listApps),
        ]);
        if (stop) return;
        setStatus(st);
        setDeps(dp);
        setEvs(ev);
        setMetas(am);
      } catch (e) {
        if (!stop) fail(e);
      } finally {
        if (!stop) setLoading(false);
      }
    }
    load();
    const t = setInterval(load, 5000);
    return () => {
      stop = true;
      clearInterval(t);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [online]);

  return { status, deps, evs, metas, loading };
}
