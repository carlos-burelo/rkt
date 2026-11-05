import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, ArrowRight, Check } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { createApp, CreateAppIn, detectApp, buildApp, deployAppAsync, DetectOut, listGithubRepos, GithubRepo } from "./api";
import { deploymentLog, getDeployment } from "../deployments/api";
import { SLUG, slugOf, looksGit, type Origin, type Step } from "./createDialog.model";
import { CreateTrack } from "./CreateTrack";
import { OriginStep, DetailsStep, ReviewStep } from "./CreateSteps";
import { useI18n } from "../../shared/i18n";

// Alta guiada por pasos: origen → detalles → revisión. Una sola acción
// primaria por paso; el backend infiere lo que falta (cfgfile.CreateApp).
export function CreateDialog({
  open,
  setOpen,
  onCreated,
  onOpen,
  goDomains,
  fail,
}: {
  open: boolean;
  setOpen: (b: boolean) => void;
  onCreated: (name: string) => void;
  onOpen: (name: string) => void;
  goDomains: () => void;
  fail: (e: unknown) => void;
}) {
  const { t } = useI18n();
  const [step, setStep] = useState<Step>(1);
  const [origin, setOrigin] = useState<Origin>("git");
  const [repo, setRepo] = useState("");
  const [ghRepos, setGhRepos] = useState<GithubRepo[] | null>(null);
  const [repoPick, setRepoPick] = useState(false);
  const [repoQ, setRepoQ] = useState("");
  const [name, setName] = useState("");
  const [nameTouched, setNameTouched] = useState(false);
  const [command, setCommand] = useState("");
  const [dir, setDir] = useState("");
  const [domain, setDomain] = useState("");
  const [port, setPort] = useState("");
  const [auto, setAuto] = useState(true);
  const [advanced, setAdvanced] = useState(false);
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState(false);
  const [plan, setPlan] = useState<DetectOut | null>(null);
  const [planBusy, setPlanBusy] = useState(false);
  const [buildNow, setBuildNow] = useState(true);
  // seguimiento en vivo post-creación (estilo Vercel)
  const [track, setTrack] = useState<{ id: number; app: string; kind: "build" | "deploy" } | null>(null);
  const [trackLog, setTrackLog] = useState("");
  const [trackStatus, setTrackStatus] = useState("running");

  // nombre derivado mientras el usuario no lo escriba
  const suggested = useMemo(() => slugOf(origin === "git" ? repo : dir), [origin, repo, dir]);

  const toggleRepos = () => {
    const next = !repoPick;
    setRepoPick(next);
    if (next && ghRepos === null) {
      listGithubRepos().then(setGhRepos).catch(fail);
    }
  };
  const reposFiltered = (ghRepos ?? []).filter((r) => {
    const q = repoQ.trim().toLowerCase();
    return q === "" || r.full_name.toLowerCase().includes(q) || r.owner.toLowerCase().includes(q);
  });
  useEffect(() => {
    if (!nameTouched) setName(suggested);
  }, [suggested, nameTouched]);

  // polling de estado + log mientras la salida está en curso
  useEffect(() => {
    if (!track) return;
    let alive = true;
    const tick = () => {
      Promise.all([getDeployment(track.id), deploymentLog(track.id, 500)])
        .then(([dep, log]) => {
          if (!alive) return;
          setTrackLog(log);
          setTrackStatus(dep.status);
        })
        .catch(() => {});
    };
    tick();
    const t = window.setInterval(tick, 1000);
    return () => {
      alive = false;
      window.clearInterval(t);
    };
  }, [track]);

  function reset() {
    setStep(1);
    setOrigin("git");
    setRepo("");
    setName("");
    setNameTouched(false);
    setCommand("");
    setDir("");
    setDomain("");
    setPort("");
    setAuto(true);
    setAdvanced(false);
    setTried(false);
    setBusy(false);
    setTrack(null);
    setTrackLog("");
    setTrackStatus("running");
  }

  const errs = {
    repo: origin === "git" && repo.trim() !== "" && !looksGit(repo) ? t("create.error.repoUrl") : "",
    name: !SLUG.test(name) ? t("create.error.name") : "",
    dir: (origin === "local" || origin === "external") && dir.trim() === "" ? t("create.error.dir") : "",
    command: origin === "manual" && command.trim() === "" ? t("create.error.command") : "",
    gitRepo: origin === "git" && repo.trim() === "" ? t("create.error.gitRepo") : "",
  };
  const step2ok = !errs.repo && !errs.name && !errs.dir && !errs.command && !errs.gitRepo;

  function next() {
    if (step === 2) {
      setTried(true);
      if (!step2ok) return;
      setTried(false);
    }
    if (step < 3) {
      const target = (step + 1) as Step;
      setStep(target);
      if (target === 3) {
        setPlan(null);
        if (origin === "local" && dir.trim() !== "") {
          setPlanBusy(true);
          detectApp(dir.trim())
            .then(setPlan)
            .catch(() => setPlan(null))
            .finally(() => setPlanBusy(false));
        }
      }
    }
  }

  function submit() {
    if (origin === "external") {
      const input: CreateAppIn = {
        name: name.trim() || undefined,
        runtime: "external",
        dir: dir.trim() || undefined,
        domains: domain.trim() ? [domain.trim()] : undefined,
        port: port.trim() === "" ? undefined : Number(port),
      };
      setBusy(true);
      createApp(input)
        .then((app) => {
          setBusy(false);
          onCreated(app);
          setOpen(false);
          reset();
          onOpen(app);
        })
        .catch((e) => {
          setBusy(false);
          fail(e);
        });
      return;
    }
    const input: CreateAppIn = {
      repo: origin === "git" ? repo.trim() : undefined,
      name: name.trim() || undefined,
      command: command.trim() || undefined,
      dir: dir.trim() || undefined,
      domains: domain.trim() ? [domain.trim()] : undefined,
      port: port.trim() === "" ? undefined : Number(port),
      auto: origin === "git" ? auto : false,
    };
    const wantBuild = buildNow && origin !== "manual";
    setBusy(true);
    createApp(input)
      .then((app) => {
        setBusy(false);
        onCreated(app);
        if (!wantBuild) {
          setOpen(false);
          reset();
          onOpen(app);
          return;
        }
        // seguimiento en vivo: guarda el id y muestra el log en el diálogo
        const job = origin === "local" ? buildApp(app) : deployAppAsync(app);
        job
          .then((id) => {
            setTrackLog("");
            setTrackStatus("running");
            setTrack({ id, app, kind: origin === "local" ? "build" : "deploy" });
          })
          .catch((e) => {
            setOpen(false);
            reset();
            onOpen(app);
            fail(e);
          });
      })
      .catch((e) => {
        setBusy(false);
        fail(e);
      });
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o: boolean) => {
        if (!o) {
          setOpen(false);
          reset();
        }
      }}
    >
      <DialogContent className="gap-4 sm:max-w-lg">
        {track ? (
          <CreateTrack
            app={track.app}
            kind={track.kind}
            status={trackStatus}
            log={trackLog}
            onClose={() => {
              setOpen(false);
              reset();
            }}
            onOpen={() => {
              const app = track.app;
              setOpen(false);
              reset();
              onOpen(app);
            }}
          />
        ) : (
          <>
        <DialogHeader>
          <div className="flex items-center gap-2 pr-8">
            <span aria-hidden className="h-1 w-4 bg-signal" />
            <span className="micro">{t("create.title")}</span>
            <span className="flex-1" />
            <span className="tnum font-mono text-[9px] text-muted-foreground">{t("create.step", { step })}</span>
          </div>
          <DialogTitle className="mt-1 text-[18px] tracking-[-0.02em]">
            {t(step === 1 ? "create.step1Title" : step === 2 ? "create.step2Title" : "create.step3Title")}
          </DialogTitle>
          <DialogDescription className="font-mono text-[10px] leading-relaxed">
            {t(step === 1 ? "create.step1Desc" : step === 2 ? "create.step2Desc" : "create.step3Desc")}
          </DialogDescription>
        </DialogHeader>

        {step === 1 && <OriginStep origin={origin} setOrigin={setOrigin} />}

        {step === 2 && (
          <DetailsStep
            origin={origin}
            repo={repo}
            setRepo={setRepo}
            ghRepos={ghRepos}
            repoPick={repoPick}
            toggleRepos={toggleRepos}
            setRepoPick={setRepoPick}
            repoQ={repoQ}
            setRepoQ={setRepoQ}
            reposFiltered={reposFiltered}
            name={name}
            setName={setName}
            setNameTouched={setNameTouched}
            domain={domain}
            setDomain={setDomain}
            command={command}
            setCommand={setCommand}
            dir={dir}
            setDir={setDir}
            auto={auto}
            setAuto={setAuto}
            advanced={advanced}
            setAdvanced={setAdvanced}
            port={port}
            setPort={setPort}
            errs={errs}
            tried={tried}
            next={next}
          />
        )}

        {step === 3 && (
          <ReviewStep
            origin={origin}
            name={name}
            repo={repo}
            dir={dir}
            command={command}
            domain={domain}
            auto={auto}
            port={port}
            plan={plan}
            planBusy={planBusy}
            buildNow={buildNow}
            setBuildNow={setBuildNow}
          />
        )}

        <div className="flex flex-wrap items-center gap-2 border-t border-border pt-3">
          {step > 1 && (
            <Button variant="ghost" size="sm" onClick={() => setStep((s) => (s - 1) as Step)} disabled={busy}>
              <ArrowLeft className="h-3 w-3" /> {t("common.back")}
            </Button>
          )}
          {step === 1 && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setOpen(false);
                goDomains();
              }}
            >
              {t("create.goDomains")}
            </Button>
          )}
          <span className="flex-1" />
          {step < 3 ? (
            <Button size="sm" onClick={next}>
              {t("common.next")} <ArrowRight className="h-3 w-3" />
            </Button>
          ) : (
            <Button size="sm" onClick={submit} disabled={busy || !name.trim()}>
              <Check className="h-3 w-3" /> {t(busy ? "create.creating" : "create.submit")}
            </Button>
          )}
        </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
