import { api } from "../../shared/lib/http";
import { AuditEntry, User, UserSession } from "../../shared/lib/types";

export async function listUsers(): Promise<User[]> {
  return api<User[]>("/api/v1/users");
}

export async function createUser(name: string, password: string, role: string): Promise<void> {
  await api("/api/v1/users", { method: "POST", body: JSON.stringify({ name, password, role }) });
}

export async function setUser(name: string, patch: { role?: string; disabled?: boolean }): Promise<void> {
  await api(`/api/v1/users/${encodeURIComponent(name)}`, { method: "PUT", body: JSON.stringify(patch) });
}

export async function deleteUser(name: string): Promise<void> {
  await api(`/api/v1/users/${encodeURIComponent(name)}`, { method: "DELETE" });
}

export async function changePassword(name: string, current: string, next: string): Promise<string> {
  const res = await api<{ token?: string }>(`/api/v1/users/${encodeURIComponent(name)}/password`, {
    method: "PUT",
    body: JSON.stringify({ current, next }),
  });
  return res.token ?? "";
}

export async function listSessions(name: string): Promise<{ sessions: UserSession[]; current: string }> {
  return api<{ sessions: UserSession[]; current: string }>(`/api/v1/users/${encodeURIComponent(name)}/sessions`);
}

export async function revokeSession(name: string, id: string): Promise<void> {
  await api(`/api/v1/users/${encodeURIComponent(name)}/sessions/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function listAudit(): Promise<AuditEntry[]> {
  return api<AuditEntry[]>("/api/v1/audit");
}
