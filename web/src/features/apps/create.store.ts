import { create } from "zustand";
import { buildApp, createApp, deployAppAsync, detectApp, detectRepo, listGithubRepos, type CreateAppIn, type DetectOut, type GithubRepo } from "./api";
import { deploymentLog, getDeployment } from "../deployments/api";
import { looksGit, SLUG, slugOf, type Origin, type Step } from "./createDialog.model";

// Wizard de alta: todo el estado y la máquina de pasos vive aquí.
// Los pasos (Origin/Details/Review/Track) solo leen y disparan acciones.
interface CreateState {
  step: Step;
  origin: Origin;
  repo: string;
  ghRepos: GithubRepo[] | null;
  repoPick: boolean;
  repoQ: string;
  name: string;
  nameTouched: boolean;
  command: string;
  dir: string;
  domain: string;
  port: string;
  auto: boolean;
  advanced: boolean;
  busy: boolean;
  tried: boolean;
  plan: DetectOut | null;
  planBusy: boolean;
  // manualRequired: git sin plan (el motivo real va en detectError, sin
  // adivinar: nogit/noclone/noplan/noresponse). manualFor recuerda para
  // qué repo se pidió (sin reintentos automáticos al insistir en Next).
  manualRequired: boolean;
  manualFor: string | null;
  detectError: { reason: string; detail?: string } | null;
  buildNow: boolean;
  track: { id: number; app: string; kind: "build" | "deploy" } | null;
  trackLog: string;
  trackStatus: string;
  set: (p: Partial<CreateState>) => void;
  reset: () => void;
  toggleRepos: (fail: (e: unknown) => void) => void;
  suggested: () => string;
  next: () => void;
  retryDetect: () => void;
  submit: (cbs: { onCreated: (n: string) => void; onOpen: (n: string) => void; fail: (e: unknown) => void }) => void;
  closeTrack: (onOpen: (n: string) => void) => void;
}

let trackTimer: ReturnType<typeof setInterval> | null = null;

function stopTrack() {
  try {
    if (trackTimer) clearInterval(trackTimer);
  } catch {
    /* ignorar */
  }
  trackTimer = null;
}

const initial = {
  step: 1 as Step,
  origin: "git" as Origin,
  repo: "",
  ghRepos: null as GithubRepo[] | null,
  repoPick: false,
  repoQ: "",
  name: "",
  nameTouched: false,
  command: "",
  dir: "",
  domain: "",
  port: "",
  auto: true,
  advanced: false,
  busy: false,
  tried: false,
  plan: null as DetectOut | null,
  planBusy: false,
  manualRequired: false,
  manualFor: null as string | null,
  detectError: null as { reason: string; detail?: string } | null,
  buildNow: true,
  track: null as { id: number; app: string; kind: "build" | "deploy" } | null,
  trackLog: "",
  trackStatus: "running",
};

export const useCreate = create<CreateState>()((set, get) => ({
  ...initial,
  set: (p) => set(p),
  reset: () => {
    stopTrack();
    set({ ...initial });
  },
  toggleRepos: (fail) => {
    const next = !get().repoPick;
    set({ repoPick: next });
    if (next && get().ghRepos === null) {
      listGithubRepos().then((ghRepos) => set({ ghRepos })).catch(fail);
    }
  },
  suggested: () => {
    const { origin, repo, dir } = get();
    return slugOf(origin === "git" ? repo : dir);
  },
  next: () => {
    const s = get();
    if (s.planBusy) return; // detección en curso: el segundo click espera
    if (s.step === 2) {
      set({ tried: true });
      if (!step2ok(s)) return;
      set({ tried: false });
    }
    if (s.step < 3) {
      // Sin git en el servidor nada git-based avanza, ni con comando
      // manual (el deploy tampoco podría clonar). Instalar git + Reintentar.
      if (s.step === 2 && s.origin === "git" && s.detectError?.reason === "nogit" && s.manualFor === s.repo.trim()) return;
      // Git sin comando: detectar el tipo en el repo antes de avanzar.
      // Sin plan no se adivina: se enseña el motivo real con reintento.
      // Sin reintentos automáticos: si ya se pidió para este repo,
      // quedarse quieto.
      if (s.step === 2 && s.origin === "git" && s.command.trim() === "") {
        const repo = s.repo.trim();
        if (s.manualRequired && s.manualFor === repo) return;
        get().retryDetect();
        return;
      }
      if (s.step === 2 && s.origin === "git") {
        set({ manualRequired: false, manualFor: null });
      }
      const target = (s.step + 1) as Step;
      set({ step: target, plan: s.origin === "git" ? s.plan : null });
      if (target === 3 && s.origin === "local" && s.dir.trim() !== "") {
        set({ planBusy: true });
        detectApp(s.dir.trim())
          .then((plan) => set({ plan }))
          .catch(() => set({ plan: null }))
          .finally(() => set({ planBusy: false }));
      }
    }
  },
  // retryDetect vuelve a detectar el repo del paso 2 (tras instalar git,
  // arreglar la URL o la red). Sin plan enseña el motivo, sin adivinar.
  retryDetect: () => {
    const s = get();
    if (s.step !== 2 || s.origin !== "git" || s.planBusy) return;
    const repo = s.repo.trim();
    if (!repo) return;
    set({ planBusy: true, manualRequired: false, manualFor: null, detectError: null });
    detectRepo(repo)
      .then((out) => {
        // El usuario pudo cambiar de repo mientras detectaba: descartar.
        if (get().repo.trim() !== repo || get().step !== 2) {
          set({ planBusy: false });
          return;
        }
        if (out && out.framework) {
          set({ plan: out, planBusy: false, manualRequired: false, manualFor: null, detectError: null, step: 3 as Step });
          return;
        }
        // Sin plan: quedarse en el paso 2 con el motivo real. null = ni
        // se pudo preguntar (red/timeout): noresponse, no noplan.
        const reason = out ? out.reason || "noplan" : "noresponse";
        set({ plan: null, planBusy: false, manualRequired: true, manualFor: repo, detectError: { reason, detail: out?.detail } });
      })
      .catch(() => {
        if (get().repo.trim() !== repo || get().step !== 2) {
          set({ planBusy: false });
          return;
        }
        set({ plan: null, planBusy: false, manualRequired: true, manualFor: repo, detectError: { reason: "noresponse" } });
      });
  },
  submit: ({ onCreated, onOpen, fail }) => {
    const s = get();
    const done = (app: string) => {
      onCreated(app);
      get().reset();
      onOpen(app);
    };
    if (s.origin === "external") {
      const input: CreateAppIn = {
        name: s.name.trim() || undefined,
        runtime: "external",
        dir: s.dir.trim() || undefined,
        domains: s.domain.trim() ? [s.domain.trim()] : undefined,
        port: s.port.trim() === "" ? undefined : Number(s.port),
      };
      set({ busy: true });
      createApp(input)
        .then(done)
        .catch((e) => {
          set({ busy: false });
          fail(e);
        });
      return;
    }
    const input: CreateAppIn = {
      repo: s.origin === "git" ? s.repo.trim() : undefined,
      name: s.name.trim() || undefined,
      command: s.command.trim() || undefined,
      dir: s.dir.trim() || undefined,
      domains: s.domain.trim() ? [s.domain.trim()] : undefined,
      port: s.port.trim() === "" ? undefined : Number(s.port),
      auto: s.origin === "git" ? s.auto : false,
    };
    const wantBuild = s.buildNow && s.origin !== "manual";
    set({ busy: true });
    createApp(input)
      .then((app) => {
        set({ busy: false });
        // Sin build: a la ficha directo. Con build: quedarse en el
        // seguimiento en vivo (el track muestra pasos + log); navegar
        // aquí lo ocultaría y el build correría sin feedback.
        if (!wantBuild) {
          done(app);
          return;
        }
        const job = s.origin === "local" ? buildApp(app) : deployAppAsync(app);
        job
          .then((id) => {
            set({ trackLog: "", trackStatus: "running", track: { id, app, kind: s.origin === "local" ? "build" : "deploy" } });
            stopTrack();
            trackTimer = setInterval(tickTrack, 1000);
            void tickTrack();
          })
          .catch((e) => {
            done(app);
            fail(e);
          });
      })
      .catch((e) => {
        set({ busy: false });
        fail(e);
      });
  },
  closeTrack: (onOpen) => {
    const app = get().track?.app;
    get().reset();
    if (app) onOpen(app);
  },
}));

async function tickTrack() {
  const { track } = useCreate.getState();
  if (!track) return;
  try {
    const [dep, log] = await Promise.all([getDeployment(track.id), deploymentLog(track.id, 500)]);
    const cur = useCreate.getState();
    if (cur.track?.id === track.id) {
      useCreate.setState({ trackLog: log, trackStatus: dep.status });
    }
  } catch {
    /* reintenta en el próximo tick */
  }
}

function step2ok(s: CreateState): boolean {
  const errs = {
    repo: s.origin === "git" && s.repo.trim() !== "" && !looksGit(s.repo),
    name: !SLUG.test(s.name),
    dir: (s.origin === "local" || s.origin === "external") && s.dir.trim() === "",
    command: s.origin === "manual" && s.command.trim() === "",
    gitRepo: s.origin === "git" && s.repo.trim() === "",
  };
  return !Object.values(errs).some(Boolean);
}
