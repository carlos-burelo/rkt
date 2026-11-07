import { api, nextDelay } from "./http";
import { useUI } from "../../app/ui";
import { useAuth } from "../../features/auth/store";
import { applyFleetEvent, useFleetBus, type FleetConn } from "../../features/overview/fleet.store";

// Conexión única SSE al bus de flota (transporte, sin estado).
// El estado vive en `features/overview/fleet.store` (zustand).
// Puerta de probe: sin backend no se abre EventSource (el navegador
// loguea cada intento fallido y no se puede silenciar). Solo con
// backend reachable se conecta el SSE; el cursor viaja en la URL.
// Backoff + watchdog half-open; probe 401 distingue sesión muerta
// de red caída.

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
let probing = false;

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

// probeAlive chequeo barato y público: cualquier respuesta HTTP = vivo.
// Silencioso (sin fail ni banner): el estado lo refleja la conexión.
async function probeAlive(): Promise<boolean> {
  try {
    const ctl = new AbortController();
    const t = setTimeout(() => ctl.abort(), 5000);
    const res = await fetch("/api/v1/auth/status", { cache: "no-store", credentials: "include", signal: ctl.signal });
    clearTimeout(t);
    return res.status < 500;
  } catch {
    return false;
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
    scheduleEnsure();
    return;
  }
  es = cur;
  lastEvent = Date.now();

  cur.addEventListener("hello", (e) => {
    lastEvent = Date.now();
    try {
      const m = JSON.parse((e as MessageEvent).data) as { seq: number };
      if (typeof m.seq === "number" && m.seq >= 0) {
        if (m.seq < loadCursor()) {
          // El servidor reinició bajo nuestro cursor: rebobinar a 0 y
          // reconectar para recibir snapshots (si no, vacío eterno).
          saveCursor(0);
          connect();
          return;
        }
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
    scheduleEnsure();
  };
}

// ensure: solo abre el SSE con backend reachable. Sin backend,
// backoff de probes silenciosos (sin el spam de EventSource en consola).
function ensure() {
  if (stopped || probing) return;
  probing = true;
  closeES();
  void probeAlive().then((up) => {
    probing = false;
    if (stopped) return;
    if (up) {
      connect();
      return;
    }
    fails++;
    if (fails >= 3) {
      setConn("offline");
    } else {
      setConn("retrying");
    }
    scheduleEnsure();
  });
}

function closeES() {
  try {
    es?.close();
  } catch {
    /* ignorar */
  }
  es = null;
}

function scheduleEnsure() {
  if (stopped) return;
  if (retryTimer) clearTimeout(retryTimer);
  retryTimer = setTimeout(ensure, nextDelay(fails));
}

function scheduleRetry() {
  if (stopped) return;
  fails++;
  if (fails >= 3) {
    // Reachable pero el stream falla: ¿sesión muerta? Un probe decide.
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
  scheduleEnsure();
}

function watch() {
  if (stopped) return;
  if (Date.now() - lastEvent > WATCHDOG_MS) {
    setConn("retrying");
    ensure();
  }
}

// subscribeFleet une una vista al bus (refcount: una conexión total).
export function subscribeFleet(): () => void {
  subs++;
  if (subs === 1) {
    stopped = false;
    probing = false;
    fails = 0;
    setConn("retrying");
    ensure();
    timer = setInterval(watch, 15000);
  }
  return () => {
    subs = Math.max(0, subs - 1);
    if (subs === 0) {
      stopped = true;
      probing = false;
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
