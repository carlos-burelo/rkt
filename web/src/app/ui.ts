import { create } from "zustand";
import { toast } from "sonner";
import { Deployment } from "../shared/lib/types";
import { appAction } from "../features/apps/api";
import { deploymentLog, retryDeployment } from "../features/deployments/api";
import { useAuth } from "../features/auth/store";
import { t } from "../shared/i18n";
import { UnauthorizedError } from "../shared/lib/http";

interface UI {
  online: boolean;
  palette: boolean;
  createOpen: boolean;
  appFilter: string;
  follow: boolean;
  sel: Deployment | null;
  buildLog: string;
  err: string;
  setOnline: (b: boolean) => void;
  setAppFilter: (s: string) => void;
  setFollow: (b: boolean) => void;
  setPalette: (b: boolean) => void;
  setCreateOpen: (b: boolean) => void;
  setSel: (d: Deployment | null) => void;
  fail: (e: unknown) => void;
  act: (app: string, a: "restart" | "deploy" | "rollback" | "start" | "stop") => Promise<void>;
  retry: (id: number) => void;
}

// UI efímera global (no datos: cada ruta carga lo suyo).
export const useUI = create<UI>()((set) => ({
  online: navigator.onLine,
  palette: false,
  createOpen: false,
  appFilter: "",
  follow: true,
  sel: null,
  buildLog: "",
  err: "",
  setOnline: (b) => set({ online: b }),
  setAppFilter: (s) => set({ appFilter: s }),
  setFollow: (b) => set({ follow: b }),
  setPalette: (b) => set({ palette: b }),
  setCreateOpen: (b) => set({ createOpen: b }),
  setSel: (d) => {
    set({ sel: d });
    if (d) {
      set({ buildLog: t("common.loading") });
      deploymentLog(d.id)
        .then((buildLog) => set({ buildLog }))
        .catch((e) => set({ buildLog: e instanceof Error ? e.message : t("shared.log.none") }));
    }
  },
  fail: (e) => {
    if (e instanceof UnauthorizedError) {
      useAuth.getState().setWho(null);
      return;
    }
    set({ err: e instanceof Error ? e.message : t("common.error") });
  },
  act: async (app, a) => {
    try {
      await appAction(app, a);
      toast.success(t("shared.action.appOk", { action: a, app }));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t("common.error"));
    }
  },
  retry: (id) => {
    retryDeployment(id)
      .then((rev) => {
        toast.success(t("shared.action.retryOk", { rev: rev.slice(0, 8) }));
        set({ sel: null });
      })
      .catch((e) => toast.error(e instanceof Error ? e.message : t("common.error")));
  },
}));
