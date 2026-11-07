import { create } from "zustand";
import { addRoute, delRoute, putRoute, listRoutes } from "./api";
import { useUI } from "../../app/ui";
import type { Route } from "../../shared/lib/types";

// Rutas host→target: lista + mutaciones. Formularios quedan locales.
interface RoutesState {
  routes: Route[];
  addOpen: boolean;
  setAddOpen: (b: boolean) => void;
  load: () => Promise<void>;
  add: (hosts: string[], target: string) => Promise<void>;
  retarget: (host: string, target: string) => Promise<void>;
  remove: (host: string) => Promise<void>;
}

export const useRoutes = create<RoutesState>()((set, get) => ({
  routes: [],
  addOpen: false,
  setAddOpen: (addOpen) => set({ addOpen }),
  load: async () => {
    try {
      set({ routes: await listRoutes() });
    } catch (e) {
      useUI.getState().fail(e);
    }
  },
  add: async (hosts, target) => {
    await addRoute(hosts, target);
    await get().load();
  },
  retarget: async (host, target) => {
    await putRoute(host, target);
    await get().load();
  },
  remove: async (host) => {
    try {
      await delRoute(host);
    } catch (e) {
      useUI.getState().fail(e);
    }
    await get().load();
  },
}));
