import { useCallback, useEffect, useState } from "react";
import { fetchProxy, ProxyRoute } from "./api";

// Caché de módulo: Dominios y Diagnóstico comparten una sola lectura.
let cache: { at: number; rows: ProxyRoute[] } | null = null;
let inflight: Promise<ProxyRoute[]> | null = null;
const TTL_MS = 5000;

async function load(force = false): Promise<ProxyRoute[]> {
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

export function useProxyStatus() {
  const [rows, setRows] = useState<ProxyRoute[] | null>(cache?.rows ?? null);
  const [err, setErr] = useState("");

  const reload = useCallback(async (force = true) => {
    setErr("");
    try {
      setRows(await load(force));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setRows([]);
    }
  }, []);

  useEffect(() => {
    let alive = true;
    load()
      .then((r) => {
        if (alive) setRows(r);
      })
      .catch((e) => {
        if (alive) {
          setErr(e instanceof Error ? e.message : String(e));
          setRows([]);
        }
      });
    return () => {
      alive = false;
    };
  }, []);

  return { rows, err, reload };
}
