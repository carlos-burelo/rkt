import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { PathInput } from "../../shared/components/PathPicker";
import { DomainSuggest } from "../../shared/components/DomainSuggest";
import { TechIcon } from "../../shared/components/TechIcon";
import { Field, ORIGINS, type Origin } from "./createDialog.model";
import type { DetectOut, GithubRepo } from "./api";
import { useI18n } from "../../shared/i18n";

export interface StepErrors {
  repo: string;
  name: string;
  dir: string;
  command: string;
  gitRepo: string;
}

// Paso 1: origen de la app.
export function OriginStep({
  origin,
  setOrigin,
}: {
  origin: Origin;
  setOrigin: (o: Origin) => void;
}) {
  const { t } = useI18n();
  return (
    <div className="space-y-1.5">
      {ORIGINS.map((o) => {
        const on = origin === o.id;
        const Icon = o.icon;
        return (
          <button
            key={o.id}
            type="button"
            onClick={() => setOrigin(o.id)}
            aria-pressed={on}
            className={`flex w-full items-start gap-3 border px-3 py-2.5 text-left transition-colors ${
              on ? "border-signal/70 bg-secondary/40" : "border-border hover:border-input hover:bg-accent/40"
            }`}
          >
            <span
              className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center border ${
                on ? "border-signal/60 text-signal" : "border-border text-muted-foreground"
              }`}
            >
              <Icon className="h-3.5 w-3.5" />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block text-[12px] font-medium">{t(o.labelKey)}</span>
              <span className="mt-0.5 block font-mono text-[10px] leading-relaxed text-muted-foreground">{t(o.hintKey)}</span>
            </span>
            <span className={`mt-1 h-1.5 w-1.5 shrink-0 ${on ? "bg-signal" : "bg-border"}`} aria-hidden />
          </button>
        );
      })}
    </div>
  );
}

// Paso 2: detalles editables.
export function DetailsStep({
  origin,
  repo,
  setRepo,
  ghRepos,
  repoPick,
  toggleRepos,
  setRepoPick,
  repoQ,
  setRepoQ,
  reposFiltered,
  name,
  setName,
  setNameTouched,
  domain,
  setDomain,
  command,
  setCommand,
  dir,
  setDir,
  auto,
  setAuto,
  advanced,
  setAdvanced,
  port,
  setPort,
  errs,
  tried,
  manualRequired,
  next,
}: {
  origin: Origin;
  repo: string;
  setRepo: (v: string) => void;
  ghRepos: GithubRepo[] | null;
  repoPick: boolean;
  toggleRepos: () => void;
  setRepoPick: (v: boolean) => void;
  repoQ: string;
  setRepoQ: (v: string) => void;
  reposFiltered: GithubRepo[];
  name: string;
  setName: (v: string) => void;
  setNameTouched: (v: boolean) => void;
  domain: string;
  setDomain: (v: string) => void;
  command: string;
  setCommand: (v: string) => void;
  dir: string;
  setDir: (v: string) => void;
  auto: boolean;
  setAuto: (v: boolean) => void;
  advanced: boolean;
  setAdvanced: (v: boolean) => void;
  port: string;
  setPort: (v: string) => void;
  errs: StepErrors;
  tried: boolean;
  manualRequired: boolean;
  next: () => void;
}) {
  const { t } = useI18n();
  return (
    <div className="space-y-3">
      {origin === "git" && (
        <Field label={t("create.repo")} hint={t("create.repoHint")} error={tried ? errs.gitRepo || errs.repo : ""}>
          <div className="flex items-center gap-1">
            <Input
              autoFocus
              value={repo}
              onChange={(e) => setRepo(e.target.value)}
              placeholder={t("create.repoPlaceholder")}
              className="font-mono text-[11px]"
              onKeyDown={(e) => {
                if (e.key === "Enter") next();
              }}
            />
            <Button type="button" size="sm" variant="outline" onClick={toggleRepos}>
              {t("create.github")}
            </Button>
          </div>
          <span className="font-mono text-[9px] normal-case text-muted-foreground/70">
            {t("create.repoCloneHint")}
          </span>
          {repoPick && (
            <div className="mt-1 border border-border bg-card/40">
              <div className="border-b border-border p-1.5">
                <Input
                  value={repoQ}
                  onChange={(e) => setRepoQ(e.target.value)}
                  placeholder={t("create.repoSearch")}
                  className="h-7 font-mono text-[10px]"
                  autoFocus
                />
              </div>
              <div className="max-h-52 overflow-auto">
                {ghRepos === null && (
                  <p className="px-2 py-3 font-mono text-[9px] text-muted-foreground">{t("create.repoLoading")}</p>
                )}
                {ghRepos !== null && reposFiltered.length === 0 && (
                  <p className="px-2 py-3 font-mono text-[9px] text-muted-foreground">
                    {t("create.repoEmpty")}
                  </p>
                )}
                {reposFiltered.map((r) => (
                  <button
                    key={r.full_name}
                    type="button"
                    onClick={() => {
                      setRepo(`https://github.com/${r.full_name}.git`);
                      setRepoPick(false);
                    }}
                    className="flex w-full items-center gap-2 border-b border-border/50 px-2 py-1.5 text-left last:border-0 hover:bg-accent/40"
                  >
                    <span className="min-w-0 flex-1 truncate font-mono text-[10px]">{r.full_name}</span>
                    <span className="shrink-0 font-mono text-[8px] text-muted-foreground">{r.default_branch}</span>
                    {r.private && (
                      <span className="shrink-0 border border-warn/40 px-1 font-mono text-[8px] text-warn">{t("create.private")}</span>
                    )}
                  </button>
                ))}
              </div>
            </div>
          )}
        </Field>
      )}
      {origin === "git" && manualRequired && (
        <div className="border border-warn/40 bg-card/40 px-3 py-2.5">
          <p className="font-mono text-[10px] leading-relaxed text-warn">{t("create.noRemotePlan")}</p>
          <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label={t("create.command")} hint={t("create.commandOverride")} error={tried ? errs.command : ""}>
              <PathInput
                value={command}
                onChange={setCommand}
                mode="file"
                placeholder={t("create.commandPlaceholder")}
                ariaLabel={t("create.command")}
              />
            </Field>
            <Field label={t("create.dir")} hint={t("create.dirHint")}>
              <PathInput
                value={dir}
                onChange={setDir}
                mode="dir"
                placeholder={t("create.dirPlaceholder")}
                ariaLabel={t("create.dir")}
              />
            </Field>
          </div>
        </div>
      )}
      {origin === "local" && (
        <Field label={t("create.dir")} hint={t("create.dirHint")} error={tried ? errs.dir : ""}>
          <PathInput
            value={dir}
            onChange={setDir}
            mode="dir"
            placeholder={t("create.dirPlaceholder")}
            ariaLabel={t("create.dir")}
          />
          <span className="font-mono text-[9px] normal-case text-muted-foreground/70">
            {t("create.dirInferHint")}
          </span>
        </Field>
      )}
      {origin === "external" && (
        <Field label={t("create.repoDir")} hint={t("create.repoDirHint")} error={tried ? errs.dir : ""}>
          <PathInput
            value={dir}
            onChange={setDir}
            mode="dir"
            placeholder={t("create.repoDirPlaceholder")}
            ariaLabel={t("create.repoDir")}
          />
          <span className="font-mono text-[9px] normal-case text-muted-foreground/70">
            {t("create.externalHint")}
          </span>
        </Field>
      )}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field label={t("create.name")} hint={t("create.nameHint")} error={tried ? errs.name : ""}>
          <Input
            value={name}
            onChange={(e) => {
              setNameTouched(true);
              setName(e.target.value);
            }}
            placeholder={t("create.namePlaceholder")}
            className="font-mono text-[11px]"
          />
        </Field>
        <Field label={t("create.domain")} hint={t("create.domainHint")}>
          <Input
            value={domain}
            onChange={(e) => setDomain(e.target.value)}
            placeholder={t("create.domainPlaceholder")}
            className="font-mono text-[11px]"
          />
        </Field>
      </div>

      <DomainSuggest name={name} onPick={setDomain} />

      {origin === "manual" && (
        <Field label={t("create.command")} hint={t("create.commandHint")} error={tried ? errs.command : ""}>
          <PathInput
            value={command}
            onChange={setCommand}
            mode="file"
            placeholder={t("create.commandPlaceholder")}
            ariaLabel={t("create.command")}
          />
        </Field>
      )}

      {origin === "git" && (
        <label className="flex items-center gap-2 font-mono text-[10px] text-muted-foreground">
          <Checkbox checked={auto} onCheckedChange={(v) => setAuto(v === true)} />
          {t("create.autoDeploy")}
        </label>
      )}

      <button
        type="button"
        onClick={() => setAdvanced(!advanced)}
        className="font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase hover:text-foreground"
        aria-expanded={advanced}
      >
        {t(advanced ? "create.hideAdvanced" : "create.advanced")}
      </button>

      {advanced && (
        <div className="grid grid-cols-1 gap-3 border border-border bg-card/40 p-3 sm:grid-cols-2">
          {origin !== "external" && (
            <Field label={t("create.command")} hint={t("create.commandOverride")}>
              <PathInput
                value={command}
                onChange={setCommand}
                mode="file"
                placeholder={t("create.commandPlaceholder")}
                ariaLabel={t("create.command")}
              />
            </Field>
          )}
          <Field label={t("create.dir")} hint={t("create.dirDefault")}>
            <PathInput
              value={dir}
              onChange={setDir}
              mode="dir"
              placeholder={t("create.dirPlaceholder")}
              ariaLabel={t("create.dir")}
            />
          </Field>
          <Field label={t("create.port")} hint={t("common.optional")}>
            <Input
              value={port}
              onChange={(e) => setPort(e.target.value.replace(/\D/g, ""))}
              inputMode="numeric"
              placeholder={t("create.portPlaceholder")}
              className="tnum font-mono text-[11px]"
            />
          </Field>
        </div>
      )}
    </div>
  );
}

// Paso 3: revisión y opción de compilar/desplegar al crear.
export function ReviewStep({
  origin,
  name,
  repo,
  dir,
  command,
  domain,
  auto,
  port,
  plan,
  planBusy,
  buildNow,
  setBuildNow,
}: {
  origin: Origin;
  name: string;
  repo: string;
  dir: string;
  command: string;
  domain: string;
  auto: boolean;
  port: string;
  plan: DetectOut | null;
  planBusy: boolean;
  buildNow: boolean;
  setBuildNow: (v: boolean) => void;
}) {
  const { t } = useI18n();
  return (
    <div className="space-y-3">
      {origin === "local" && (
        <div className="border border-border bg-card/40">
          <div className="flex flex-wrap items-center gap-2 border-b border-border bg-secondary/20 px-3 py-1.5">
            <span aria-hidden className="h-1.5 w-1.5 bg-signal" />
            <span className="micro">{t("create.analysis")}</span>
            <span className="flex-1" />
            {planBusy ? (
              <span className="font-mono text-[9px] text-muted-foreground">{t("create.analyzing")}</span>
            ) : plan ? (
              <span className="flex items-center gap-1.5 border border-signal/40 bg-signal/10 px-1.5 py-0.5 font-mono text-[9px] tracking-[0.08em] uppercase text-signal">
                <TechIcon kind={plan.framework} size={14} />
                {plan.framework}
                {plan.pm ? ` · ${plan.pm}` : ""}
              </span>
            ) : (
              <span className="font-mono text-[9px] text-warn">{t("create.noPlan")}</span>
            )}
          </div>
          <div className="space-y-1.5 px-3 py-2">
            {plan && (
              <>
                <p className="font-mono text-[10px] text-muted-foreground">
                  {t("create.install")}: {plan.install.length ? plan.install.join(" · ") : t("create.nothing")}
                </p>
                <p className="font-mono text-[10px] text-muted-foreground">
                  {t("create.build")}: {plan.build.length ? plan.build.join(" · ") : t("create.nothing")}
                </p>
                <p className="truncate font-mono text-[10px] text-muted-foreground">
                  {plan.static_dir
                    ? `${t("create.static")}: ${plan.static_abs}`
                    : `${t("create.start")}: ${plan.start.join(" ") || "-"}`}
                </p>
                {(plan.evidence?.length ?? 0) > 0 && (
                  <p className="truncate font-mono text-[10px] text-muted-foreground">
                    {t("create.evidence")}: {plan.evidence?.join(" · ")}
                  </p>
                )}
                {plan.node_version && (
                  <p className="font-mono text-[10px] text-muted-foreground">
                    {t("create.nodeVersion")}: {plan.node_version}
                  </p>
                )}
                {plan.workspace && (
                  <p className="font-mono text-[10px] text-warn">{t("create.workspace")}</p>
                )}
              </>
            )}
            {!plan && !planBusy && (
              <p className="font-mono text-[10px] leading-relaxed text-muted-foreground">
                {t("create.noPlanHint")}
              </p>
            )}
          </div>
        </div>
      )}

      {origin === "git" && (
        <p className="font-mono text-[10px] leading-relaxed text-muted-foreground">
          {t("create.gitPlanHint")}
        </p>
      )}

      {origin === "external" && (
        <p className="border border-border bg-card/40 px-3 py-2 font-mono text-[10px] leading-relaxed text-muted-foreground">
          {t("create.externalReview")}
        </p>
      )}

      <div className="border border-border">
        {[
          [t("create.review.origin"), ORIGINS.find((o) => o.id === origin) ? t(ORIGINS.find((o) => o.id === origin)!.labelKey) : origin],
          [t("create.review.name"), name],
          ...(repo.trim() ? [[t("create.review.repo"), repo.trim()]] : []),
          ...(origin === "git" && plan?.framework ? [[t("create.review.detected"), plan.framework]] : []),
          ...(dir.trim() ? [[t("create.review.dir"), dir.trim()]] : []),
          [
            t("create.review.command"),
            origin === "external"
              ? t("create.review.externalCommand")
              : command.trim() || (plan?.start.join(" ") ?? t("create.review.detectedCommand")),
          ],
          [t("create.review.domain"), domain.trim() || t("create.review.noDomain")],
          ...(origin === "git" ? [[t("create.review.auto"), t(auto ? "create.review.autoYes" : "create.review.autoNo")]] : []),
          ...(origin === "external" && port.trim() ? [[t("create.review.port"), port.trim()]] : []),
        ].map(([k, v]) => (
          <div key={k} className="flex items-baseline gap-3 border-b border-border/60 px-3 py-2 last:border-0">
            <span className="micro w-28 shrink-0">{k}</span>
            <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{v}</span>
          </div>
        ))}
      </div>

      {(origin === "git" || origin === "local") && (
        <label className="flex items-center gap-2 font-mono text-[10px] text-muted-foreground">
          <Checkbox checked={buildNow} onCheckedChange={(v) => setBuildNow(v === true)} />
          {t(origin === "git" ? "create.deployNow" : "create.buildNow")}
        </label>
      )}
    </div>
  );
}
