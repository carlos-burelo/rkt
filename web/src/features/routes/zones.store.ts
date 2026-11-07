import { create } from "zustand";
import { addRoute, delRoute, listZones, type Zones } from "./api";
import { useUI } from "../../app/ui";

// Zonas locales: agrupación DNS sobre /api/v1/routes. Inputs del
// formulario quedan en el componente; la lista vive aquí.
interface ZonesState {
  zones: Zones | null;
  load: () => Promise<void>;
  createZone: (host: string, target: string) => Promise<void>;
  addSub: (host: string, target: string) => Promise<void>;
  remove: (host: string) => Promise<void>;
}

export const useZones = create<ZonesState>()((set, get) => ({
  zones: null,
  load: async () => {
    try {
      set({ zones: await listZones() });
    } catch (e) {
      useUI.getState().fail(e);
    }
  },
  createZone: async (host, target) => {
    await addRoute([host], target);
    await get().load();
  },
  addSub: async (host, target) => {
    await addRoute([host], target);
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
