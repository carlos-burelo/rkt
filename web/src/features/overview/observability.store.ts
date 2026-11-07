import { create } from "zustand";
import { fetchObs, type Obs } from "./api";

// Observabilidad agregada: ventana + serie. Recarga al cambiar ventana.
interface ObservabilityState {
  hours: number;
  data: Obs | null;
  setHours: (h: number) => void;
  load: () => void;
}

export const useObservability = create<ObservabilityState>()((set, get) => ({
  hours: 6,
  data: null,
  setHours: (hours) => {
    set({ hours, data: null });
    get().load();
  },
  load: () => {
    const { hours } = get();
    fetchObs(hours)
      .then((data) => {
        if (get().hours === hours) set({ data });
      })
      .catch(() => {
        if (get().hours === hours) set({ data: null });
      });
  },
}));
