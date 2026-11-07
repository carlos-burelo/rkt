import { create } from "zustand";
import { Me } from "../../shared/lib/types";
import { logout, me, authStatus } from "./api";
import { getSettings } from "../settings/api";

interface AuthState {
  who: Me | null;
  brand: string;
  setupRequired: boolean | null;
  authed: boolean;
  isAdmin: boolean;
  canWrite: boolean;
  /** boot terminó (status + me asentados): la puerta puede decidir sin rebote */
  ready: boolean;
  boot: () => Promise<void>;
  setWho: (m: Me | null) => void;
  doLogout: () => void;
}

// Auth global: sesión, setup inicial, marca. Un hogar.
export const useAuth = create<AuthState>()((set) => ({
  who: null,
  brand: "rkt",
  setupRequired: null,
  authed: false,
  isAdmin: false,
  canWrite: false,
  ready: false,
  boot: async () => {
    try {
      const s = await authStatus();
      set({ setupRequired: s.setup_required });
      if (s.setup_required) {
        set({ ready: true });
        return;
      }
      try {
        const m = await me();
        set({ who: m, authed: true, isAdmin: m.role === "admin", canWrite: m.role !== "viewer" });
        getSettings()
          .then((gs) => {
            if (gs.site_name) set({ brand: gs.site_name });
          })
          .catch(() => {});
      } catch {
        set({ who: null, authed: false, isAdmin: false, canWrite: false });
      }
    } catch {
      set({ setupRequired: false, who: null, authed: false });
    }
    set({ ready: true });
  },
  setWho: (m) =>
    set((s) => ({
      who: m,
      authed: m !== null,
      // Tras setup, setupRequired seguía true y la puerta devolvía a /setup
      // hasta recargar. Con sesión válida el setup ya no aplica.
      setupRequired: m !== null ? false : s.setupRequired,
      isAdmin: m?.role === "admin",
      canWrite: m?.role === "admin" || m?.role === "operator",
    })),
  doLogout: () => {
    logout().finally(() =>
      set({ who: null, authed: false, isAdmin: false, canWrite: false }),
    );
  },
}));
