import { create } from "zustand";
import { addRoute, delRoute, domainDetail, putRoute, listRoutes, type DomainDetail } from "./api";
import { useUI } from "../../app/ui";
import type { Route } from "../../shared/lib/types";

// Rutas host→target: lista + mutaciones + detalle cacheado por host
// (el drawer y los badges TLS de cada fila comparten una lectura).
interface RoutesState {
  routes: Route[];
  addOpen: boolean;
  details: Record<string, { data: DomainDetail | null; error: string } | undefined>;
  setAddOpen: (b: boolean) => void;
  load: () => Promise<void>;
  add: (hosts: string[], target: string) => Promise<void>;
  retarget: (host: string, target: string) => Promise<void>;
  remove: (host: string) => Promise<void>;
  detail: (host: string) => Promise<void>;
}

const inflight = new Map<string, Promise<void>>();

export const useRoutes = create<RoutesState>()((set, get) => ({
  routes: [],
  addOpen: false,
  details: {},
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
  detail: (host) => {
    if (get().details[host] !== undefined || inflight.has(host)) {
      return inflight.get(host) ?? Promise.resolve();
    }
    const p = domainDetail(host)
      .then((d) => {
        useRoutes.setState((s) => ({ details: { ...s.details, [host]: { data: d, error: "" } } }));
      })
      .catch((e) => {
        useRoutes.setState((s) => ({
          details: { ...s.details, [host]: { data: null, error: e instanceof Error ? e.message : String(e) } },
        }));
      })
      .finally(() => {
        inflight.delete(host);
      });
    inflight.set(host, p);
    return p;
  },
}));
