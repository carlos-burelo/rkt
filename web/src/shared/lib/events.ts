import { api, nextDelay } from "./http";
import { useUI } from "../../app/ui";
import { useAuth } from "../../features/auth/store";
import { applyFleetEvent, useFleetBus, type FleetConn } from "../../features/overview/fleet.store";

// Conexión única SSE al bus de flota (transporte, sin estado).
// El estado vive en `features/overview/fleet.store` (zustand).
// Reconexión manual: el cursor viaja en la URL (el retry nativo
// repetiría el cursor viejo). Backoff + watchdog half-open; probe 401
// distingue sesión muerta de red caída.

const CURSOR_KEY = "rkt.fleet.cursor";
const WATCHDOG_MS = 45000;
const TOPICS = "status,deployments,events,metas";

function loadCursor(): number {
  try {
    return Number(sessionStorage.getItem(CURSOR_KEY) ?? 0) || 0;
  } catch {
    return 0;
  }
}

function saveCursor(seq: number) {
  try {
    sessionStorage.setItem(CURSOR_KEY, String(seq));
  } catch {
    /* almacenamiento no disponible */
  }
}

let es: EventSource | null = null;
let subs = 0;
let fails = 0;
let lastEvent = 0;
let timer: ReturnType<typeof setInterval> | null = null;
let retryTimer: ReturnType<typeof setTimeout> | null = null;
let stopped = false;

function setConn(c: FleetConn) {
  useFleetBus.getState().setConn(c);
  if (c === "live") {
    fails = 0;
    useUI.getState().setOnline(true);
    useUI.getState().clearErr();
  } else if (c === "offline") {
    useUI.getState().setOnline(false);
  }
}

async function probeAuth(): Promise<boolean> {
  try {
    await api<{ user?: string }>("/api/v1/auth/me");
    return true;
  } catch (e) {
    if (e instanceof Error && e.message.includes("No autorizado")) return false;
    return true; // red caída, no sesión muerta
  }
}

function connect() {
  if (stopped) return;
  try {
    es?.close();
  } catch {
    /* ignorar */
  }
  const cursor = loadCursor();
  let cur: EventSource;
  try {
    cur = new EventSource(`/api/v1/fleet/stream?topics=${TOPICS}&cursor=${cursor}`);
  } catch {
    scheduleRetry();
    return;
  }
  es = cur;
  lastEvent = Date.now();

  cur.addEventListener("hello", (e) => {
    lastEvent = Date.now();
    try {
      const m = JSON.parse((e as MessageEvent).data) as { seq: number };
      if (typeof m.seq === "number" && m.seq >= 0) {
        saveCursor(m.seq);
      }
    } catch {
      /* hello malformado: seguir */
    }
    setConn("live");
  });

  // Snapshot: estado completo, siempre aplica (no filtra por seq).
  cur.addEventListener("snapshot", (e) => {
    lastEvent = Date.now();
    try {
      const m = JSON.parse((e as MessageEvent).data) as { topic?: string; data?: unknown };
      if (typeof m.topic === "string") {
        useFleetBus.getState().applySnapshot(m.topic, m.data);
      }
    } catch {
      /* frame corrupto: ignorar */
    }
    setConn("live");
  });

  // Live: filtra por seq (duplicados o viejos se ignoran).
  const onData = (e: Event) => {
    lastEvent = Date.now();
    try {
      const m = JSON.parse((e as MessageEvent).data) as { seq?: number; topic?: string; data?: unknown };
      if (typeof m.seq === "number" && typeof m.topic === "string") {
        saveCursor(m.seq);
        applyFleetEvent(m.seq, m.topic, m.data);
      }
    } catch {
      /* frame corrupto: ignorar */
    }
    setConn("live");
  };
  for (const t of ["status", "deployments", "events", "metas"]) {
    cur.addEventListener(t, onData);
  }
  cur.addEventListener("ping", () => {
    lastEvent = Date.now();
  });
  cur.onerror = () => {
    if (stopped || es !== cur) return;
    setConn("retrying");
    scheduleRetry();
  };
}

function scheduleRetry() {
  if (stopped) return;
  try {
    es?.close();
  } catch {
    /* ignorar */
  }
  es = null;
  fails++;
  if (fails >= 3) {
    // ¿Red o sesión? Un probe decide: 401 => login, resto => offline.
    void probeAuth().then((ok) => {
      if (stopped) return;
      if (!ok) {
        useAuth.getState().setWho(null);
        return;
      }
      setConn("offline");
    });
  } else {
    setConn("retrying");
  }
  if (retryTimer) clearTimeout(retryTimer);
  retryTimer = setTimeout(connect, nextDelay(fails));
}

function watch() {
  if (stopped) return;
  if (Date.now() - lastEvent > WATCHDOG_MS) {
    setConn("retrying");
    connect();
  }
}

// subscribeFleet une una vista al bus (refcount: una conexión total).
export function subscribeFleet(): () => void {
  subs++;
  if (subs === 1) {
    stopped = false;
    fails = 0;
    setConn("retrying");
    connect();
    timer = setInterval(watch, 15000);
  }
  return () => {
    subs = Math.max(0, subs - 1);
    if (subs === 0) {
      stopped = true;
      if (timer) clearInterval(timer);
      timer = null;
      if (retryTimer) clearTimeout(retryTimer);
      retryTimer = null;
      try {
        es?.close();
      } catch {
        /* ignorar */
      }
      es = null;
    }
  };
}
