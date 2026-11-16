import { create } from "zustand";
import { toast } from "sonner";
import { appAction } from "../features/apps/api";
import { retryDeployment } from "../features/deployments/api";
import { useAuth } from "../features/auth/store";
import { t } from "../shared/i18n";
import { UnauthorizedError } from "../shared/lib/http";
import { errorToast } from "../shared/components/feedback";

interface UI {
  online: boolean;
  palette: boolean;
  appFilter: string;
  follow: boolean;
  setOnline: (b: boolean) => void;
  setAppFilter: (s: string) => void;
  setFollow: (b: boolean) => void;
  setPalette: (b: boolean) => void;
  fail: (e: unknown, friendly?: string, id?: string) => void;
  act: (app: string, a: "restart" | "deploy" | "rollback" | "start" | "stop") => Promise<void>;
  retry: (id: number) => void;
}

// UI efímera global (no datos: cada ruta carga lo suyo). Los errores salen
// por sonner con botón de copiar (errorToast), nunca en banner.
export const useUI = create<UI>()((set) => ({
  online: navigator.onLine,
  palette: false,
  appFilter: "",
  follow: true,
  setOnline: (b) => set({ online: b }),
  setAppFilter: (s) => set({ appFilter: s }),
  setFollow: (b) => set({ follow: b }),
  setPalette: (b) => set({ palette: b }),
  fail: (e, friendly, id) => {
    if (e instanceof UnauthorizedError) {
      useAuth.getState().setWho(null);
      return;
    }
    const msg = e instanceof Error ? e.message : t("common.error");
    errorToast(friendly ?? msg, msg, id ? { id } : undefined);
  },
  act: async (app, a) => {
    try {
      await appAction(app, a);
      toast.success(t("shared.action.appOk", { action: a, app }));
    } catch (e) {
      errorToast(e instanceof Error ? e.message : t("common.error"));
    }
  },
  retry: (id) => {
    retryDeployment(id)
      .then((rev) => {
        toast.success(t("shared.action.retryOk", { rev: rev.slice(0, 8) }));
      })
      .catch((e) => errorToast(e instanceof Error ? e.message : t("common.error")));
  },
}));
