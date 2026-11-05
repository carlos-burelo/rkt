import { t } from "../i18n";

// UnauthorizedError permite reaccionar sin comparar el texto localizado.
export class UnauthorizedError extends Error {}

// HTTP base api/v1. Sesión por cookie httpOnly (credentials en cada fetch).
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
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
  const res = await fetch(path, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
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
// Devuelve close; EventSource usa la cookie de sesión.
export function streamLog(
  url: string,
  onLog: (text: string, append: boolean) => void,
  onEnd?: (status: string) => void,
): () => void {
  const es = new EventSource(url);
  const read = (e: MessageEvent) => JSON.parse(e.data) as { log?: string; chunk?: string; status?: string };
  es.addEventListener("init", (e) => onLog(read(e as MessageEvent).log ?? "", false));
  es.addEventListener("chunk", (e) => onLog(read(e as MessageEvent).chunk ?? "", true));
  es.addEventListener("end", (e) => {
    onEnd?.(read(e as MessageEvent).status ?? "");
    es.close();
  });
  es.onerror = () => es.close();
  return () => es.close();
}
