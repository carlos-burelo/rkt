import { create } from "zustand";
import { api, nextDelay } from "./http";
import { useUI } from "../../app/ui";
import { useAuth } from "../../features/auth/store";
import type { AppMeta, Deployment, RktEvent, Status } from "./types";

// Estado vivo de la flota: un solo dueño (fleet bus), N lectores.
// El poll se eliminó: el servidor publica solo cuando algo cambia y el
// stream trae snapshot + replay por cursor. Sin solape, sin banners falsos.

export type FleetConn = "live" | "retrying" | "offline";

interface FleetBus {
  status: Status | null;
  deps: Deployment[];
  evs: RktEvent[];
  metas: AppMeta[];
  seq: number;
  conn: FleetConn;
  loading: boolean;
  applySnapshot: (topic: string, data: unknown) => void;
  setConn: (c: FleetConn) => void;
}

const CURSOR_KEY = "rkt.fleet.cursor";
const WATCHDOG_MS = 45000;

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

function asStatus(data: unknown): Status | null {
  if (!data || typeof data !== "object" || !Array.isArray((data as Status).apps)) return null;
  return data as Status;
}

function asArray<T>(data: unknown): T[] {
  return Array.isArray(data) ? (data as T[]) : [];
}

export const useFleetBus = create<FleetBus>()((set) => ({
  status: null,
  deps: [],
  evs: [],
  metas: [],
  seq: 0,
  conn: "retrying",
  loading: true,
  applySnapshot: (topic, data) =>
    set(() => {
      if (topic === "status") {
        const st = asStatus(data);
        return st ? { status: st, loading: false } : {};
      }
      if (topic === "deployments") return { deps: asArray<Deployment>(data), loading: false };
      if (topic === "events") return { evs: asArray<RktEvent>(data), loading: false };
      if (topic === "metas") return { metas: asArray<AppMeta>(data) };
      return {};
    }),
  setConn: (conn) => set({ conn }),
}));

// Aplica un publish (estado completo por tópico) y avanza el cursor.
function applyEvent(seq: number, topic: string, data: unknown) {
  saveCursor(seq);
  const st = useFleetBus.getState();
  if (seq <= st.seq) return; // duplicado o viejo
  useFleetBus.setState({ seq });
  st.applySnapshot(topic, data);
}

// Manager: una sola conexión SSE para toda la UI. Reconexión manual
// (el cursor viaja en la URL: el retry nativo repetiría el cursor viejo)
// con backoff; watchdog caza half-open; probe 401 distingue sesión
// muerta de red caída.
let es: EventSource | null = null;
let subs = 0;
let fails = 0;
let lastEvent = 0;
let timer: ReturnType<typeof setInterval> | null = null;
let retryTimer: ReturnType<typeof setTimeout> | null = null;
let stopped = false;

const TOPICS = "status,deployments,events,metas";

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
        applyEvent(m.seq, m.topic, m.data);
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
