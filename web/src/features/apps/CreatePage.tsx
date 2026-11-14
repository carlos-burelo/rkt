import { useEffect } from "react";
import { useNavigate } from "react-router";
import { ArrowLeft, ArrowRight, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "../../shared/components/patterns";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import { SLUG, looksGit } from "./createDialog.model";
import { OriginStep, DetailsStep, ReviewStep } from "./CreateSteps";
import { CreateTrack } from "./CreateTrack";
import { useCreate } from "./create.store";

// Alta de app como página dedicada (estilo Vercel): foco total, stepper
// visible, espacio para plan y seguimiento en vivo. Reemplaza al modal.
const STEPS = [1, 2, 3] as const;

export function CreatePage() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const fail = useUI((u) => u.fail);
  const s = useCreate();

  useEffect(() => {
    useCreate.getState().reset();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!s.nameTouched) s.set({ name: s.suggested() });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [s.repo, s.dir, s.origin, s.nameTouched]);

  const errs = {
    repo: s.origin === "git" && s.repo.trim() !== "" && !looksGit(s.repo) ? t("create.error.repoUrl") : "",
    name: !SLUG.test(s.name) ? t("create.error.name") : "",
    dir: (s.origin === "local" || s.origin === "external") && s.dir.trim() === "" ? t("create.error.dir") : "",
    command: (s.origin === "manual" || (s.origin === "git" && s.manualRequired)) && s.command.trim() === "" ? t("create.error.command") : "",
    gitRepo: s.origin === "git" && s.repo.trim() === "" ? t("create.error.gitRepo") : "",
  };

  const openApp = (name: string) => navigate(`/app/${name}`);
  const done = (name: string) => openApp(name);

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6">
      <PageHeader
        title={t("create.title")}
        description={t(s.step === 1 ? "create.step1Desc" : s.step === 2 ? "create.step2Desc" : "create.step3Desc")}
        meta={
          <span className="tnum font-mono text-[10px] text-muted-foreground">
            {t("create.step", { step: s.step })}
          </span>
        }
      />

      {!s.track && (
        <ol className="flex items-center gap-0 border border-border bg-card/35" aria-label={t("create.stepsAria")}>
          {STEPS.map((n, i) => {
            const active = s.step === n;
            const past = s.step > n;
            return (
              <li key={n} className={`flex flex-1 items-center gap-2 px-3 py-2.5 ${i > 0 ? "border-l border-border" : ""} ${active ? "bg-accent/40" : ""}`}>
                <span
                  aria-hidden
                  className={`tnum flex h-5 w-5 shrink-0 items-center justify-center font-mono text-[10px] ${
                    active ? "bg-signal font-bold text-black" : past ? "bg-ok/20 text-ok" : "bg-secondary text-muted-foreground"
                  }`}
                >
                  {past ? "✓" : n}
                </span>
                <span className={`min-w-0 truncate text-[12px] ${active ? "font-medium text-foreground" : "text-muted-foreground"}`}>
                  {t(n === 1 ? "create.step1Title" : n === 2 ? "create.step2Title" : "create.step3Title")}
                </span>
              </li>
            );
          })}
        </ol>
      )}

      {s.track ? (
        <CreateTrack
          app={s.track.app}
          kind={s.track.kind}
          status={s.trackStatus}
          log={s.trackLog}
          onClose={() => {
            s.reset();
            navigate("/overview");
          }}
          onOpen={() => s.closeTrack(openApp)}
        />
      ) : (
        <section className="border border-border bg-card/35 p-4 md:p-6">
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
              manualRequired={s.manualRequired}
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

          <div className="mt-6 flex flex-wrap items-center gap-2 border-t border-border pt-4">
            {s.step > 1 && (
              <Button variant="ghost" size="sm" onClick={() => s.set({ step: (s.step - 1) as typeof s.step })} disabled={s.busy}>
                <ArrowLeft className="h-3 w-3" /> {t("common.back")}
              </Button>
            )}
            <span className="flex-1" />
            {s.step < 3 ? (
              <Button size="sm" onClick={() => s.next()}>
                {t("common.next")} <ArrowRight className="h-3 w-3" />
              </Button>
            ) : (
              <Button
                size="sm"
                onClick={() => s.submit({ onCreated: done, onOpen: openApp, fail })}
                disabled={s.busy || !s.name.trim()}
              >
                <Check className="h-3 w-3" /> {t(s.busy ? "create.creating" : "create.submit")}
              </Button>
            )}
          </div>
        </section>
      )}
    </div>
  );
}
