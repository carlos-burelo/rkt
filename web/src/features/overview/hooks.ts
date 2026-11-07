import { useEffect } from "react";
import type { AppMeta, Deployment, RktEvent, Status } from "../../shared/lib/types";
import { subscribeFleet, useFleetBus } from "../../shared/lib/events";

export interface Fleet {
  status: Status | null;
  deps: Deployment[];
  evs: RktEvent[];
  metas: AppMeta[];
  loading: boolean;
}

// useFleet lee el bus único (una conexión SSE para toda la UI).
// Sin polling: el servidor publica solo al cambiar + snapshot al conectar.
export function useFleet(): Fleet {
  useEffect(() => subscribeFleet(), []);
  const status = useFleetBus((s) => s.status);
  const deps = useFleetBus((s) => s.deps);
  const evs = useFleetBus((s) => s.evs);
  const metas = useFleetBus((s) => s.metas);
  const loading = useFleetBus((s) => s.loading);
  return { status, deps, evs, metas, loading };
}
