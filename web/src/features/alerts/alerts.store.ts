import { create } from "zustand";
import {
  addChannel, addRule, delChannel, delRule, listChannels, listRules, testRule, updateRule,
  type AlertChannel, type AlertRule, type RuleIn,
} from "./api";
import { useUI } from "../../app/ui";

// Reglas + canales: una carga, N vistas. Los formularios quedan locales.
interface AlertsState {
  rules: AlertRule[];
  channels: AlertChannel[];
  load: () => Promise<void>;
  save: (id: number, payload: RuleIn) => Promise<void>;
  toggle: (r: AlertRule) => Promise<void>;
  remove: (id: number) => Promise<void>;
  test: (id: number) => Promise<string>;
  add: (payload: RuleIn) => Promise<void>;
  addChannel: (name: string, url: string) => Promise<void>;
  removeChannel: (id: number) => Promise<void>;
}

export const useAlerts = create<AlertsState>()((set, get) => {
  const reload = async () => {
    try {
      const [rules, channels] = await Promise.all([listRules(), listChannels().catch(() => [] as AlertChannel[])]);
      set({ rules, channels });
    } catch (e) {
      useUI.getState().fail(e);
    }
  };
  return {
    rules: [],
    channels: [],
    load: reload,
    save: async (id, payload) => {
      await updateRule(id, payload);
      await reload();
    },
    toggle: async (r) => {
      try {
        await updateRule(r.id, {
          app: r.app, kind: r.kind, threshold: r.threshold,
          window_min: r.window_min, enabled: !r.enabled, channel_id: r.channel_id,
        });
      } catch (e) {
        useUI.getState().fail(e);
      }
      await reload();
    },
    remove: async (id) => {
      try {
        await delRule(id);
      } catch (e) {
        useUI.getState().fail(e);
      }
      await reload();
    },
    test: async (id) => {
      try {
        return await testRule(id);
      } catch (e) {
        useUI.getState().fail(e);
        return "";
      }
    },
    addChannel: async (name, url) => {
      await addChannel(name, url);
      await reload();
    },
    removeChannel: async (id) => {
      try {
        await delChannel(id);
      } catch (e) {
        useUI.getState().fail(e);
      }
      await reload();
    },
    add: async (payload) => {
      await addRule(payload);
      await reload();
    },
  };
});
