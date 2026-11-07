import { useEffect } from "react";
import { ArrowLeft, ArrowRight, Check } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { SLUG, looksGit } from "./createDialog.model";
import { CreateTrack } from "./CreateTrack";
import { OriginStep, DetailsStep, ReviewStep } from "./CreateSteps";
import { useCreate } from "./create.store";
import { useI18n } from "../../shared/i18n";

// Alta guiada por pasos: el store lleva la máquina; esto solo enmarca.
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
  const s = useCreate();

  // nombre derivado mientras el usuario no lo escriba
  useEffect(() => {
    if (!s.nameTouched) s.set({ name: s.suggested() });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [s.repo, s.dir, s.origin, s.nameTouched]);

  const errs = {
    repo: s.origin === "git" && s.repo.trim() !== "" && !looksGit(s.repo) ? t("create.error.repoUrl") : "",
    name: !SLUG.test(s.name) ? t("create.error.name") : "",
    dir: (s.origin === "local" || s.origin === "external") && s.dir.trim() === "" ? t("create.error.dir") : "",
    command: s.origin === "manual" && s.command.trim() === "" ? t("create.error.command") : "",
    gitRepo: s.origin === "git" && s.repo.trim() === "" ? t("create.error.gitRepo") : "",
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o: boolean) => {
        if (!o) {
          setOpen(false);
          s.reset();
        }
      }}
    >
      <DialogContent className="gap-4 sm:max-w-lg">
        {s.track ? (
          <CreateTrack
            app={s.track.app}
            kind={s.track.kind}
            status={s.trackStatus}
            log={s.trackLog}
            onClose={() => {
              setOpen(false);
              s.reset();
            }}
            onOpen={() => s.closeTrack(setOpen, onOpen)}
          />
        ) : (
          <>
        <DialogHeader>
          <div className="flex items-center gap-2 pr-8">
            <span aria-hidden className="h-1 w-4 bg-signal" />
            <span className="micro">{t("create.title")}</span>
            <span className="flex-1" />
            <span className="tnum font-mono text-[9px] text-muted-foreground">{t("create.step", { step: s.step })}</span>
          </div>
          <DialogTitle className="mt-1 text-[18px] tracking-[-0.02em]">
            {t(s.step === 1 ? "create.step1Title" : s.step === 2 ? "create.step2Title" : "create.step3Title")}
          </DialogTitle>
          <DialogDescription className="font-mono text-[10px] leading-relaxed">
            {t(s.step === 1 ? "create.step1Desc" : s.step === 2 ? "create.step2Desc" : "create.step3Desc")}
          </DialogDescription>
        </DialogHeader>

        {s.step === 1 && <OriginStep origin={s.origin} setOrigin={(origin) => s.set({ origin })} />}

        {s.step === 2 && (
          <DetailsStep
            origin={s.origin}
            repo={s.repo}
            setRepo={(repo) => s.set({ repo })}
            ghRepos={s.ghRepos}
            repoPick={s.repoPick}
            toggleRepos={() => s.toggleRepos(fail)}
            setRepoPick={(repoPick) => s.set({ repoPick })}
            repoQ={s.repoQ}
            setRepoQ={(repoQ) => s.set({ repoQ })}
            reposFiltered={(s.ghRepos ?? []).filter((r) => {
              const q = s.repoQ.trim().toLowerCase();
              return q === "" || r.full_name.toLowerCase().includes(q) || r.owner.toLowerCase().includes(q);
            })}
            name={s.name}
            setName={(name) => s.set({ name })}
            setNameTouched={(nameTouched) => s.set({ nameTouched })}
            domain={s.domain}
            setDomain={(domain) => s.set({ domain })}
            command={s.command}
            setCommand={(command) => s.set({ command })}
            dir={s.dir}
            setDir={(dir) => s.set({ dir })}
            auto={s.auto}
            setAuto={(auto) => s.set({ auto })}
            advanced={s.advanced}
            setAdvanced={(advanced) => s.set({ advanced })}
            port={s.port}
            setPort={(port) => s.set({ port })}
            errs={errs}
            tried={s.tried}
            next={() => s.next()}
          />
        )}

        {s.step === 3 && (
          <ReviewStep
            origin={s.origin}
            name={s.name}
            repo={s.repo}
            dir={s.dir}
            command={s.command}
            domain={s.domain}
            auto={s.auto}
            port={s.port}
            plan={s.plan}
            planBusy={s.planBusy}
            buildNow={s.buildNow}
            setBuildNow={(buildNow) => s.set({ buildNow })}
          />
        )}

        <div className="flex flex-wrap items-center gap-2 border-t border-border pt-3">
          {s.step > 1 && (
            <Button variant="ghost" size="sm" onClick={() => s.set({ step: (s.step - 1) as typeof s.step })} disabled={s.busy}>
              <ArrowLeft className="h-3 w-3" /> {t("common.back")}
            </Button>
          )}
          {s.step === 1 && (
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
          {s.step < 3 ? (
            <Button size="sm" onClick={() => s.next()}>
              {t("common.next")} <ArrowRight className="h-3 w-3" />
            </Button>
          ) : (
            <Button size="sm" onClick={() => s.submit({ onCreated, onOpen, setOpen, fail })} disabled={s.busy || !s.name.trim()}>
              <Check className="h-3 w-3" /> {t(s.busy ? "create.creating" : "create.submit")}
            </Button>
          )}
        </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
