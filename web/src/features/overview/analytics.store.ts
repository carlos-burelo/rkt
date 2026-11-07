import { create } from "zustand";
import { fetchAnalytics, type Analytics } from "./api";

// Analytics por host y rango: filtros + serie + error local.
interface AnalyticsState {
  host: string;
  hosts: string[];
  days: number;
  data: Analytics | null;
  err: string;
  setHost: (h: string) => void;
  setDays: (d: number) => void;
  load: () => void;
}

export const useAnalyticsPage = create<AnalyticsState>()((set, get) => ({
  host: "",
  hosts: [],
  days: 7,
  data: null,
  err: "",
  setHost: (host) => {
    set({ host, data: null, err: "" });
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
    set({ data: null, err: "" });
    fetchAnalytics(host, days)
      .then((data) => {
        const s = get();
        if (s.host === host && s.days === days) set({ data });
      })
      .catch((e) => {
        const s = get();
        if (s.host === host && s.days === days) {
          set({ err: e instanceof Error ? e.message : "error" });
        }
      });
  },
}));
