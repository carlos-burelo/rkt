import { create } from "zustand";
import { listUsers, listAudit, createUser, deleteUser, setUser } from "./api";
import { useUI } from "../../app/ui";
import type { AuditEntry, User } from "../../shared/lib/types";

// Directorio de usuarios + auditoría: una carga, N vistas.
interface UsersState {
  users: User[];
  audit: AuditEntry[];
  load: () => Promise<void>;
  create: (name: string, password: string, role: string) => Promise<void>;
  remove: (name: string) => Promise<void>;
  toggle: (name: string, disabled: boolean) => Promise<void>;
}

export const useUsers = create<UsersState>()((set, get) => ({
  users: [],
  audit: [],
  load: async () => {
    try {
      set({ users: await listUsers() });
      listAudit()
        .then((audit) => set({ audit }))
        .catch(() => {});
    } catch (e) {
      useUI.getState().fail(e);
    }
  },
  create: async (name, password, role) => {
    await createUser(name, password, role);
    await get().load();
  },
  remove: async (name) => {
    try {
      await deleteUser(name);
    } catch (e) {
      useUI.getState().fail(e);
    }
    await get().load();
  },
  toggle: async (name, disabled) => {
    try {
      await setUser(name, { disabled });
    } catch (e) {
      useUI.getState().fail(e);
    }
    await get().load();
  },
}));
