import { create } from "zustand";
import { streamLog } from "../../shared/lib/http";
import { fetchLog } from "./api";
import { deploymentLog } from "../deployments/api";
import { useUI } from "../../app/ui";
import type { Level, Stream } from "./format";

// Consola de logs: todo el estado aquí. Los efectos de suscripción viven
// en el componente; el parseo derivado en useLogLines (una sola vez).
interface LogsState {
  tab: "runtime" | "build";
  q: string;
  live: boolean;
  streamState: "live" | "retrying";
  levels: Record<Level, boolean>;
  stream: "all" | Stream;
  log: string;
  follow: boolean;
  compact: boolean;
  wrap: boolean;
  absolute: boolean;
  context: boolean;
  build: string;
  setTab: (t: "runtime" | "build") => void;
  setQ: (q: string) => void;
  setLive: (b: boolean) => void;
  toggleLevel: (l: Level) => void;
  setStream: (s: "all" | Stream) => void;
  setFollow: (b: boolean) => void;
  setCompact: (b: boolean) => void;
  setWrap: (b: boolean) => void;
  setAbsolute: (b: boolean) => void;
  setContext: (b: boolean) => void;
  setBuild: (b: string) => void;
  appendLog: (text: string) => void;
  replaceLog: (text: string) => void;
  load: () => Promise<void>;
  download: () => void;
  fetchBuild: (id: number, loadingMsg: string, noLogMsg: string) => void;
  connect: () => () => void;
  resetStream: () => void;
}

let esClose: (() => void) | null = null;

export const useLogs = create<LogsState>()((set, get) => ({
  tab: "runtime",
  q: "",
  live: true,
  streamState: "live",
  levels: { info: true, warning: true, error: true },
  stream: "all",
  log: "",
  follow: true,
  compact: true,
  wrap: false,
  absolute: true,
  context: false,
  build: "",
  setTab: (tab) => set({ tab }),
  setQ: (q) => set({ q }),
  setLive: (live) => set({ live }),
  toggleLevel: (l) => set((s) => ({ levels: { ...s.levels, [l]: !s.levels[l] } })),
  setStream: (stream) => set({ stream }),
  setFollow: (follow) => set({ follow }),
  setCompact: (compact) => set({ compact }),
  setWrap: (wrap) => set({ wrap }),
  setAbsolute: (absolute) => set({ absolute }),
  setContext: (context) => set({ context }),
  setBuild: (build) => set({ build }),
  appendLog: (text) => set((s) => ({ log: s.log + text })),
  replaceLog: (text) => set({ log: text }),
  load: async () => {
    try {
      set({ log: await fetchLog(useUI.getState().appFilter) });
    } catch (e) {
      useUI.getState().fail(e);
    }
  },
  download: () => {
    const { log } = get();
    const blob = new Blob([log], { type: "text/plain" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `${useUI.getState().appFilter || "rkt"}.log`;
    a.click();
    URL.revokeObjectURL(a.href);
  },
  fetchBuild: (id, loadingMsg, noLogMsg) => {
    set({ build: loadingMsg });
    deploymentLog(id)
      .then((build) => set({ build }))
      .catch((e) => set({ build: e instanceof Error ? e.message : noLogMsg }));
  },
  connect: () => {
    const appFilter = useUI.getState().appFilter;
    const q = `n=200${appFilter !== "" ? `&app=${encodeURIComponent(appFilter)}` : ""}`;
    esClose?.();
    esClose = streamLog(
      `/api/v1/logs/stream?${q}`,
      (text, append) => {
        if (append) get().appendLog(text);
        else get().replaceLog(text);
      },
      undefined,
      (s) => set({ streamState: s }),
    );
    return () => {
      esClose?.();
      esClose = null;
    };
  },
  resetStream: () => set({ stream: "all", streamState: "live" }),
}));
