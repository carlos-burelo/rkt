import { useEffect, useState } from "react";
import { Status, Deployment, RktEvent, AppMeta } from "../../shared/lib/types";
import { cached, nextDelay } from "../../shared/lib/http";
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

// useFleet estado compartido con poll secuencial (sin solape) y backoff:
// un fallo aislado no pinta banner; 3 seguidos marcan offline y el
// banner pasa a "sin conexión / reconectando" en vez de "Failed to fetch".
// Al volver, online + limpia error. Respuestas cacheadas 4s:
// sidebar, página y paleta montadas a la vez = una request.
export function useFleet(): Fleet {
  const clearErr = useUI((u) => u.clearErr);
  const online = useUI((u) => u.online);
  const setOnline = useUI((u) => u.setOnline);
  const [status, setStatus] = useState<Status | null>(null);
  const [deps, setDeps] = useState<Deployment[]>([]);
  const [evs, setEvs] = useState<RktEvent[]>([]);
  const [metas, setMetas] = useState<AppMeta[]>([]);
  const [loading, setLoading] = useState(true);

  // Pestaña oculta: poll lento (15s) para no gastar batería ni logs.
  const HIDDEN_MS = 15000;

  useEffect(() => {
    if (!online) return;
    let stop = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let fails = 0;
    async function load() {
      if (stop) return;
      try {
        const [st, dp, ev, am] = await Promise.all([
          cached("fleet:status", 4000, fetchStatus),
          cached("fleet:deps", 4000, () => fetchDeployments()),
          cached("fleet:evs", 4000, () => fetchEvents()),
          cached("fleet:apps", 4000, listApps),
        ]);
        if (stop) return;
        fails = 0;
        setStatus(st);
        setDeps(dp);
        setEvs(ev);
        setMetas(am);
        clearErr();
        if (!useUI.getState().online) setOnline(true);
      } catch {
        if (stop) return;
        fails++;
        if (fails >= 3) setOnline(false);
      } finally {
        if (!stop) {
          setLoading(false);
          timer = setTimeout(load, document.hidden ? HIDDEN_MS : nextDelay(fails));
        }
      }
    }
    load();
    return () => {
      stop = true;
      if (timer) clearTimeout(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [online]);

  return { status, deps, evs, metas, loading };
}
