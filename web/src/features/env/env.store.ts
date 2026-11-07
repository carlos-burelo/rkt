import { create } from "zustand";
import { useUI } from "../../app/ui";
import { getEnvIndex } from "./api";

// Índice env global (solo claves): una carga compartida por EnvAll y AppEnv.
interface EnvIndexState {
  keys: string[];
  appKeys: Record<string, string[]>;
  load: () => void;
}

let inflight: Promise<void> | null = null;

export const useEnvIndex = create<EnvIndexState>()((set) => ({
  keys: [],
  appKeys: {},
  load: () => {
    if (inflight) return;
    inflight = getEnvIndex()
      .then((idx) => {
        set({ keys: idx.global, appKeys: idx.apps });
      })
      .catch((e) => useUI.getState().fail(e))
      .finally(() => {
        inflight = null;
      });
  },
}));

// Compat: el hook ahora lee el store (una sola lectura global).
import { useEffect } from "react";

export function useGlobalEnv(): { keys: string[]; appKeys: Record<string, string[]>; reload: () => void } {
  const keys = useEnvIndex((s) => s.keys);
  const appKeys = useEnvIndex((s) => s.appKeys);
  const online = useUI((s) => s.online);
  useEffect(() => {
    if (online) useEnvIndex.getState().load();
  }, [online]);
  return { keys, appKeys, reload: () => useEnvIndex.getState().load() };
}
