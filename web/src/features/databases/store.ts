import { create } from "zustand";
import {
  getPg, listTables, describeTable,
  type PgApp, type PgTableDetail, type PgTableSchema, type PgPolicy,
} from "./api";
import { useUI } from "../../app/ui";

// Estado compartido del módulo de bases: el sidebar contextual (DbSidebar)
// y la página (Databases) leen lo mismo, igual que AppTabs/AppSidebar con
// la flota. Filas, filtros y SQL quedan locales en la página.
export type DbTab = "tablas" | "esquema" | "objetos" | "sql" | "copias";

interface DbState {
  apps: PgApp[];
  enabled: boolean;
  sel: string;
  tables: PgTableDetail[];
  table: string;
  schema: PgTableSchema | null;
  policies: PgPolicy[];
  tab: DbTab;
  setTab: (tab: DbTab) => void;
  loadApps: (keepSel?: string) => Promise<void>;
  selectApp: (app: string) => void;
  selectTable: (table: string) => void;
  refreshTables: (keep?: string) => Promise<void>;
  loadSchema: () => Promise<void>;
}

export const useDb = create<DbState>()((set, get) => ({
  apps: [],
  enabled: true,
  sel: "",
  tables: [],
  table: "",
  schema: null,
  policies: [],
  tab: "tablas",
  setTab: (tab) => set({ tab }),
  loadApps: async (keepSel) => {
    try {
      const out = await getPg();
      const want = keepSel ?? get().sel;
      const sel =
        out.apps.length && !out.apps.some((a) => a.app === want)
          ? out.apps[0].app
          : want && out.apps.length
            ? want
            : out.apps.length
              ? out.apps[0].app
              : "";
      set({ apps: out.apps, enabled: out.enabled });
      if (sel !== get().sel) {
        get().selectApp(sel);
      } else if (sel) {
        await get().refreshTables();
      }
    } catch (e) {
      useUI.getState().fail(e);
    }
  },
  selectApp: (app) => {
    set({ sel: app, table: "", tables: [], schema: null, policies: [] });
    if (app) void get().refreshTables();
  },
  selectTable: (table) => {
    set({ table });
    if (table) void get().loadSchema();
  },
  refreshTables: async (keep) => {
    const { sel, table } = get();
    if (!sel) return;
    try {
      const ts = await listTables(sel);
      const want = keep ?? table;
      const next = ts.length && !ts.some((x) => x.name === want) ? ts[0].name : want;
      set({ tables: ts, table: next && ts.length ? next : "" });
      if (next && ts.length) await get().loadSchema();
      else set({ schema: null, policies: [] });
    } catch (e) {
      useUI.getState().fail(e);
    }
  },
  loadSchema: async () => {
    const { sel, table } = get();
    if (!sel || !table) {
      set({ schema: null, policies: [] });
      return;
    }
    try {
      const d = await describeTable(sel, table);
      if (get().sel === sel && get().table === table) {
        set({ schema: d.schema, policies: d.policies });
      }
    } catch (e) {
      if (get().sel === sel && get().table === table) set({ schema: null, policies: [] });
      useUI.getState().fail(e);
    }
  },
}));
