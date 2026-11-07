import { create } from "zustand";
import { fetchUsage, type Usage } from "./api";
import { getSettings } from "../settings/api";

// Uso 30d + quotas: una carga al montar, lectura desde N vistas.
interface UsageStripState {
  u: Usage | null;
  quotas: Record<string, string>;
  load: () => void;
}

export const useUsageStrip = create<UsageStripState>()((set) => ({
  u: null,
  quotas: {},
  load: () => {
    fetchUsage(30)
      .then((u) => set({ u }))
      .catch(() => {});
    getSettings()
      .then((quotas) => set({ quotas }))
      .catch(() => {});
  },
}));
