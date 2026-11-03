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
  boot: async () => {
    try {
      const s = await authStatus();
      set({ setupRequired: s.setup_required });
      if (s.setup_required) return;
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
  },
  setWho: (m) =>
    set({
      who: m,
      authed: m !== null,
      isAdmin: m?.role === "admin",
      canWrite: m?.role === "admin" || m?.role === "operator",
    }),
  doLogout: () => {
    logout().finally(() =>
      set({ who: null, authed: false, isAdmin: false, canWrite: false }),
    );
  },
}));
