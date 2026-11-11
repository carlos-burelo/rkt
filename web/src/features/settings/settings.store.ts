import { create } from "zustand";
import { getSettings, setSetting, getGithubStatus, type GithubStatus } from "./api";
import { createToken, listTokens, revokeToken, type ApiToken } from "./api";
import { useUI } from "../../app/ui";

// Ajustes + GitHub + tokens MCP remoto: lectura y mutación centralizadas.
interface SettingsState {
  settings: Record<string, string>;
  draft: Record<string, string>;
  gh: GithubStatus | null;
  tokens: ApiToken[];
  load: () => Promise<void>;
  setDraftKey: (k: string, v: string) => void;
  save: (k: string) => Promise<void>;
  resetKey: (k: string) => Promise<void>;
  loadTokens: () => Promise<void>;
  mintToken: (name: string, role?: string) => Promise<string>;
  revoke: (id: number) => Promise<void>;
}

export const useSettings = create<SettingsState>()((set, get) => ({
  settings: {},
  draft: {},
  gh: null,
  tokens: [],
  load: async () => {
    try {
      const s = await getSettings();
      set({ settings: s, draft: s });
    } catch (e) {
      useUI.getState().fail(e);
    }
    getGithubStatus()
      .then((gh) => set({ gh }))
      .catch(() => set({ gh: null }));
  },
  setDraftKey: (k, v) => set((s) => ({ draft: { ...s.draft, [k]: v } })),
  save: async (k) => {
    const { draft, settings } = get();
    await setSetting(k, draft[k] ?? settings[k] ?? "");
    await get().load();
  },
  resetKey: async (k) => {
    try {
      await setSetting(k, "");
    } catch (e) {
      useUI.getState().fail(e);
    }
    await get().load();
  },
  loadTokens: async () => {
    set({ tokens: await listTokens() });
  },
  mintToken: async (name, role = "operator") => {
    const tok = await createToken(name.trim() || "mcp-agent", role, 24 * 90);
    await get().loadTokens();
    return tok;
  },
  revoke: async (id) => {
    await revokeToken(id);
    await get().loadTokens();
  },
}));
