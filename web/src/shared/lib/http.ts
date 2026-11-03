// HTTP base api/v1. Sesión por cookie httpOnly (credentials en cada fetch).
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (res.status === 401) {
    throw new Error("no autorizado (login)");
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error((body as { error?: string }).error ?? `http ${res.status}`);
  }
  return (await res.json()) as T;
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
