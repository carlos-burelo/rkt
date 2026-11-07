import { create } from "zustand";
import { getUsage } from "./api";
import { useUI } from "../../app/ui";
import type { AppUsage } from "../../shared/lib/types";

// Consumo medido por app: datos + auto-poll en el store (la vista solo
// lee y arranca/para por app). Interval propio, sin solape.
interface AppUsageState {
  u: AppUsage | null;
  at: Date | null;
  busy: boolean;
  auto: boolean;
  setAuto: (b: boolean) => void;
  load: (app: string) => void;
  start: (app: string) => void;
  stop: () => void;
}

let timer: ReturnType<typeof setInterval> | null = null;
let currentApp = "";

export const useAppUsage = create<AppUsageState>()((set, get) => ({
  u: null,
  at: null,
  busy: false,
  auto: true,
  setAuto: (auto) => {
    set({ auto });
    if (auto) {
      const app = currentApp;
      if (app) get().load(app);
    }
  },
  load: (app) => {
    if (!app || get().busy) return;
    set({ busy: true });
    getUsage(app)
      .then((u) => set({ u, at: new Date() }))
      .catch((e) => useUI.getState().fail(e))
      .finally(() => set({ busy: false }));
  },
  start: (app) => {
    currentApp = app;
    try {
      if (timer) clearInterval(timer);
    } catch {
      /* ignorar */
    }
    timer = null;
    set({ u: null, at: null });
    if (!useUI.getState().online) return;
    get().load(app);
    timer = setInterval(() => {
      if (get().auto && useUI.getState().online) get().load(app);
    }, 5000);
  },
  stop: () => {
    currentApp = "";
    try {
      if (timer) clearInterval(timer);
    } catch {
      /* ignorar */
    }
    timer = null;
  },
}));
