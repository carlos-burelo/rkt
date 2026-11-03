import { api } from "../../shared/lib/http";
import { Me } from "../../shared/lib/types";

export async function login(user: string, password: string): Promise<Me> {
  return api<Me>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ user, password }),
  });
}

export async function logout(): Promise<void> {
  try {
    await api<{ ok: string }>("/api/v1/auth/logout", { method: "POST" });
  } catch {
    // sesión ya inválida: igual se sale
  }
}

export async function me(): Promise<Me> {
  return api<Me>("/api/v1/auth/me");
}
