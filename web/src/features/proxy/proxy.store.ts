import { create } from "zustand";
import { fetchProxy, type ProxyRoute } from "./api";

// Diagnóstico proxy: filas + filtros compartidos (Dominios y Diagnóstico
// leen lo mismo). Una sola lectura con caché 5s.
interface ProxyState {
  rows: ProxyRoute[] | null;
  err: string;
  fState: "all" | "ok" | "down";
  fTls: "all" | "tls" | "off";
  selected: string | null;
  load: (force?: boolean) => Promise<void>;
  setFState: (f: "all" | "ok" | "down") => void;
  setFTls: (f: "all" | "tls" | "off") => void;
  setSelected: (h: string | null) => void;
}

let cache: { at: number; rows: ProxyRoute[] } | null = null;
let inflight: Promise<ProxyRoute[]> | null = null;
const TTL_MS = 5000;

async function read(force = false): Promise<ProxyRoute[]> {
  if (!force && cache && Date.now() - cache.at < TTL_MS) return cache.rows;
  if (inflight) return inflight;
  inflight = fetchProxy()
    .then((rows) => {
      cache = { at: Date.now(), rows };
      return rows;
    })
    .finally(() => {
      inflight = null;
    });
  return inflight;
}

export const useProxy = create<ProxyState>()((set) => ({
  rows: null,
  err: "",
  fState: "all",
  fTls: "all",
  selected: null,
  load: async (force = true) => {
    set({ err: "" });
    try {
      set({ rows: await read(force) });
    } catch (e) {
      set({ err: e instanceof Error ? e.message : String(e), rows: [] });
    }
  },
  setFState: (fState) => set({ fState }),
  setFTls: (fTls) => set({ fTls }),
  setSelected: (selected) => set({ selected }),
}));

// Compat: el hook anterior ahora lee el store (una sola lectura global).
import { useEffect } from "react";

export function useProxyStatus() {
  const rows = useProxy((s) => s.rows);
  const err = useProxy((s) => s.err);
  useEffect(() => {
    useProxy.getState().load(false);
  }, []);
  return { rows, err, reload: (force = true) => useProxy.getState().load(force) };
}
