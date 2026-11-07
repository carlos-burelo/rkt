import { create } from "zustand";
import type { AppMeta, Deployment, RktEvent, Status } from "../../shared/lib/types";

// Estado vivo de la flota: SOLO estado (sin transporte, sin fetch).
// El único escritor es el bus (`shared/lib/events`); N vistas leen.
// Sin polling: snapshot al conectar + publishes con seq del servidor.

export type FleetConn = "live" | "retrying" | "offline";

interface FleetBusState {
  status: Status | null;
  deps: Deployment[];
  evs: RktEvent[];
  metas: AppMeta[];
  seq: number;
  conn: FleetConn;
  loading: boolean;
  applySnapshot: (topic: string, data: unknown) => void;
  setConn: (c: FleetConn) => void;
  setSeq: (seq: number) => void;
}

function asStatus(data: unknown): Status | null {
  if (!data || typeof data !== "object" || !Array.isArray((data as Status).apps)) return null;
  return data as Status;
}

function asArray<T>(data: unknown): T[] {
  return Array.isArray(data) ? (data as T[]) : [];
}

export const useFleetBus = create<FleetBusState>()((set) => ({
  status: null,
  deps: [],
  evs: [],
  metas: [],
  seq: 0,
  conn: "retrying",
  loading: true,
  applySnapshot: (topic, data) =>
    set(() => {
      if (topic === "status") {
        const st = asStatus(data);
        return st ? { status: st, loading: false } : {};
      }
      if (topic === "deployments") return { deps: asArray<Deployment>(data), loading: false };
      if (topic === "events") return { evs: asArray<RktEvent>(data), loading: false };
      if (topic === "metas") return { metas: asArray<AppMeta>(data) };
      return {};
    }),
  setConn: (conn) => set({ conn }),
  setSeq: (seq) => set({ seq }),
}));

// Aplica un publish en vivo: filtra duplicados/viejos por seq.
export function applyFleetEvent(seq: number, topic: string, data: unknown) {
  const st = useFleetBus.getState();
  if (seq <= st.seq) return; // duplicado o viejo
  useFleetBus.setState({ seq });
  st.applySnapshot(topic, data);
}
