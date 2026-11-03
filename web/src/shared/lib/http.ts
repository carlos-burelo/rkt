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
