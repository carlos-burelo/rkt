import { t } from "../i18n";

// UnauthorizedError permite reaccionar sin comparar el texto localizado.
export class UnauthorizedError extends Error {}

// HTTP base api/v1. Sesión por cookie httpOnly (credentials en cada fetch).
// Sin timeout el fetch cuelga para siempre y los polls se apilan:
// 15s por defecto (AbortSignal.timeout), el llamante puede dar su signal.
export const API_TIMEOUT_MS = 15000;
export const POLL_BASE_MS = 5000;
export const POLL_MAX_MS = 30000;

// nextDelay backoff del poll tras `fails` fallos seguidos:
// 5s, 5s, 10s, 20s, 30s… tope 30s. Puro y testeable.
export function nextDelay(fails: number): number {
  if (fails <= 1) return POLL_BASE_MS;
  return Math.min(POLL_BASE_MS * 2 ** (fails - 1), POLL_MAX_MS);
}

function withTimeout(init?: RequestInit): RequestInit {
  if (init?.signal) return init;
  return { ...init, signal: AbortSignal.timeout(API_TIMEOUT_MS) };
}

function toError(e: unknown, res?: Response): Error {
  if (e instanceof UnauthorizedError) return e;
  if (e instanceof DOMException && e.name === "AbortError") {
    return new Error(t("shared.http.timeout"));
  }
  if (e instanceof TypeError) {
    return new Error(t("shared.http.offline"));
  }
  return e instanceof Error ? e : new Error(t("shared.http.status", { status: res?.status ?? 0 }));
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      ...withTimeout(init),
    });
  } catch (e) {
    throw toError(e);
  }
  if (res.status === 401) {
    throw new UnauthorizedError(t("shared.http.unauthorized"));
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error((body as { error?: string }).error ?? t("shared.http.status", { status: res.status }));
  }
  return (await res.json()) as T;
}

// apiResult conserva error/hint del backend en respuestas no-2xx.
// Para endpoints que necesitan el body de error (p. ej. fix hosts).
export type ApiResult<T> =
  | { ok: true; data: T }
  | { ok: false; error: string; hint?: string };

export async function apiResult<T>(path: string, init?: RequestInit): Promise<ApiResult<T>> {
  let res: Response;
  try {
    res = await fetch(path, {
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      ...withTimeout(init),
    });
  } catch (e) {
    const err = toError(e);
    return { ok: false as const, error: err.message };
  }
  const body = (await res.json().catch(() => ({}))) as {
    error?: string;
    hint?: string;
  };
  if (!res.ok) {
    return { ok: false, error: body.error ?? t("shared.http.status", { status: res.status }), hint: body.hint };
  }
  return { ok: true, data: body as T };
}

// Caché en memoria con TTL: varias rutas montadas a la vez comparten
// la misma respuesta (una sola request en vuelo por clave).
const memo = new Map<string, { exp: number; p: Promise<unknown> }>();

export function cached<T>(key: string, ms: number, fn: () => Promise<T>): Promise<T> {
  const now = Date.now();
  const hit = memo.get(key);
  if (hit && hit.exp > now) return hit.p as Promise<T>;
  const p = fn().catch((e) => {
    if (memo.get(key)?.p === p) memo.delete(key);
    throw e;
  });
  memo.set(key, { exp: now + ms, p });
  if (memo.size > 64) {
    for (const [k, v] of memo) {
      if (v.exp <= now) memo.delete(k);
    }
  }
  return p;
}

// bust invalida claves por prefijo (tras mutaciones).
export function bust(prefix: string) {
  for (const k of memo.keys()) {
    if (k.startsWith(prefix)) memo.delete(k);
  }
}

// streamLog sigue un log por SSE. onLog recibe el tail inicial
// (append=false) y luego solo los bytes nuevos (append=true).
// Reconexión nativa de EventSource (NO cerrar en onerror: eso mataba
// la consola al primer blip) + watchdog: sin eventos en 45s se
// recrea el stream (caza conexiones half-open que el browser no ve;
// el servidor manda `: ping` cada 15s pero los comentarios no generan
// eventos). onState avisa a la UI (reconectando…). Devuelve close.
export type StreamState = "live" | "retrying";

const STREAM_WATCHDOG_MS = 45000;

export function streamLog(
  url: string,
  onLog: (text: string, append: boolean) => void,
  onEnd?: (status: string) => void,
  onState?: (s: StreamState) => void,
): () => void {
  let es: EventSource | null = null;
  let lastEvent = 0;
  let closed = false;
  let timer: ReturnType<typeof setInterval> | null = null;
  const read = (e: MessageEvent) => JSON.parse(e.data) as { log?: string; chunk?: string; status?: string };
  const touch = () => {
    lastEvent = Date.now();
  };

  function connect() {
    if (closed) return;
    es?.close();
    const cur = new EventSource(url);
    es = cur;
    cur.addEventListener("init", (e) => {
      touch();
      onState?.("live");
      onLog(read(e as MessageEvent).log ?? "", false);
    });
    cur.addEventListener("chunk", (e) => {
      touch();
      onLog(read(e as MessageEvent).chunk ?? "", true);
    });
    cur.addEventListener("end", (e) => {
      touch();
      onEnd?.(read(e as MessageEvent).status ?? "");
      cur.close();
      if (es === cur) es = null;
    });
    // Error: NO cerrar (el browser reintenta solo). Solo avisar.
    // Si el error es definitivo el watchdog recrea el stream.
    cur.onerror = () => {
      if (!closed) onState?.("retrying");
    };
  }

  lastEvent = Date.now();
  connect();
  timer = setInterval(() => {
    if (closed) return;
    if (Date.now() - lastEvent > STREAM_WATCHDOG_MS) {
      onState?.("retrying");
      lastEvent = Date.now();
      connect();
    }
  }, 15000);

  return () => {
    closed = true;
    if (timer) clearInterval(timer);
    es?.close();
    es = null;
  };
}
