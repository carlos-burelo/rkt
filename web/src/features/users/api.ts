import { api } from "../../shared/lib/http";
import { User } from "../../shared/lib/types";

export async function listUsers(): Promise<User[]> {
  return api<User[]>("/api/v1/users");
}

export async function createUser(name: string, password: string, role: string): Promise<void> {
  await api("/api/v1/users", { method: "POST", body: JSON.stringify({ name, password, role }) });
}

export async function deleteUser(name: string): Promise<void> {
  await api(`/api/v1/users/${name}`, { method: "DELETE" });
}
