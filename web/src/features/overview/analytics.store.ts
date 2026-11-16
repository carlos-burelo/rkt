import { create } from "zustand";
import { fetchAnalytics, type Analytics } from "./api";
import { useUI } from "../../app/ui";

// Analytics por host y rango: filtros + serie (el error va por sonner).
interface AnalyticsState {
  host: string;
  hosts: string[];
  days: number;
  data: Analytics | null;
  setHost: (h: string) => void;
  setDays: (d: number) => void;
  load: () => void;
}

export const useAnalyticsPage = create<AnalyticsState>()((set, get) => ({
  host: "",
  hosts: [],
  days: 7,
  data: null,
  setHost: (host) => {
    set({ host, data: null });
    get().load();
  },
  setDays: (days) => {
    set({ days });
    fetchAnalytics("", days)
      .then((g) => {
        if (get().days === days) set({ hosts: g.by_host.map((h) => h.k) });
      })
      .catch(() => {});
    get().load();
  },
  load: () => {
    const { host, days } = get();
    set({ data: null });
    fetchAnalytics(host, days)
      .then((data) => {
        const s = get();
        if (s.host === host && s.days === days) set({ data });
      })
      .catch((e) => {
        const s = get();
        if (s.host === host && s.days === days) {
          useUI.getState().fail(e);
        }
      });
  },
}));
