import { create } from "zustand";
import { fetchServer, fetchServerHistory, fetchServerProcesses } from "./api";
import type { ServerPoint, ServerProcess, ServerSnapshot } from "./api";

// Monitor del host: snapshot 5s + procesos 15s. Polls con guard alive.
interface ServerState {
  snap: ServerSnapshot | null;
  history: ServerPoint[];
  procs: ServerProcess[];
  err: string;
  start: () => void;
  stop: () => void;
}

let snapTimer: ReturnType<typeof setInterval> | null = null;
let procTimer: ReturnType<typeof setInterval> | null = null;
let alive = false;

export const useServer = create<ServerState>()((set) => ({
  snap: null,
  history: [],
  procs: [],
  err: "",
  start: () => {
    stopTimers();
    alive = true;
    const loadSnap = () => {
      fetchServer()
        .then((s) => {
          if (alive) set({ snap: s, err: "" });
        })
        .catch((e) => {
          if (alive) set({ err: e instanceof Error ? e.message : "error" });
        });
      fetchServerHistory(6)
        .then((h) => {
          if (alive) set({ history: h });
        })
        .catch(() => {});
    };
    const loadProcs = () => {
      fetchServerProcesses(10)
        .then((p) => {
          if (alive) set({ procs: p });
        })
        .catch(() => {});
    };
    loadSnap();
    loadProcs();
    snapTimer = setInterval(loadSnap, 5000);
    procTimer = setInterval(loadProcs, 15000);
  },
  stop: () => {
    alive = false;
    stopTimers();
  },
}));

function stopTimers() {
  try {
    if (snapTimer) clearInterval(snapTimer);
    if (procTimer) clearInterval(procTimer);
  } catch {
    /* ignorar */
  }
  snapTimer = procTimer = null;
}
