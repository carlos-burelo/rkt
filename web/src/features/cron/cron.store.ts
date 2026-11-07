import { create } from "zustand";
import { deleteCron, listCron, runCron, type CronJob } from "./api";
import { useUI } from "../../app/ui";

// Tareas programadas: lista + diálogo de alta/edición. El formulario
// queda en el componente (CronForm); el poll vive aquí.
interface CronState {
  jobs: CronJob[];
  formOpen: boolean;
  editing: CronJob | null;
  openNew: () => void;
  openEdit: (j: CronJob) => void;
  closeForm: () => void;
  load: () => Promise<void>;
  start: () => void;
  stop: () => void;
  remove: (app: string, job: string) => Promise<void>;
  runNow: (app: string, job: string) => Promise<void>;
}

let timer: ReturnType<typeof setInterval> | null = null;

export const useCron = create<CronState>()((set, get) => ({
  jobs: [],
  formOpen: false,
  editing: null,
  openNew: () => set({ editing: null, formOpen: true }),
  openEdit: (editing) => set({ editing, formOpen: true }),
  closeForm: () => set({ formOpen: false, editing: null }),
  load: async () => {
    try {
      set({ jobs: await listCron() });
    } catch (e) {
      useUI.getState().fail(e);
    }
  },
  start: () => {
    try {
      if (timer) clearInterval(timer);
    } catch {
      /* ignorar */
    }
    get().load();
    timer = setInterval(() => {
      if (useUI.getState().online) get().load();
    }, 5000);
  },
  stop: () => {
    try {
      if (timer) clearInterval(timer);
    } catch {
      /* ignorar */
    }
    timer = null;
  },
  remove: async (app, job) => {
    try {
      await deleteCron(app, job);
    } catch (e) {
      useUI.getState().fail(e);
    }
    await get().load();
  },
  runNow: async (app, job) => {
    await runCron(app, job);
  },
}));
