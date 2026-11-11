import { useEffect, useState } from "react";
import { setup } from "./api";
import { Me } from "../../shared/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Err } from "../../shared/components/ui";
import { AccessShell } from "./AccessShell";
import { useI18n } from "../../shared/i18n";
import { fetchDoctor, type DoctorCheck } from "../server/api";
import { getSettings, setSetting, getGithubStatus, type GithubStatus } from "../settings/api";
import { fixHost } from "../routes/api";

type Step = "admin" | "red" | "tools" | "github" | "done";

// Setup por pasos: admin (obligatorio) + red/herramientas/GitHub
// omitibles. El doctor corre en el paso 3 con reparación in situ
// (bins). Todo lo omitido sigue disponible en el dashboard.
export function Setup({ onOk }: { onOk: (me: Me) => void }) {
  const { t } = useI18n();
  const [step, setStep] = useState<Step>("admin");
  const [me, setMe] = useState<Me | null>(null);
  const steps: Step[] = ["admin", "red", "tools", "github", "done"];
  const idx = steps.indexOf(step);

  return (
    <AccessShell
      title={t("system.auth.setupTitle")}
      description={t("system.auth.setupDesc")}
      footer={
        <div className="space-y-2">
          <div className="flex items-center gap-1.5" aria-label={t("system.auth.stepsAria")}>
            {steps.map((s, i) => (
              <span
                key={s}
                title={t(`system.auth.step.${s}`)}
                aria-current={i === idx ? "step" : undefined}
                className={`h-1 flex-1 border ${i < idx ? "border-signal/60 bg-signal/40" : i === idx ? "border-signal bg-signal/70" : "border-border bg-transparent"}`}
              />
            ))}
          </div>
          <p className="font-mono text-[9px] leading-relaxed text-muted-foreground/70">
            {t("system.auth.stepOf", { n: idx + 1, total: steps.length, step: t(`system.auth.step.${step}`) })}
            <br />
            {t("system.auth.setupFooterTwo")}
          </p>
        </div>
      }
    >
      {step === "admin" && <StepAdmin onDone={(m) => { setMe(m); setStep("red"); }} />}
      {step === "red" && <StepRed onNext={() => setStep("tools")} onBack={() => setStep("admin")} />}
      {step === "tools" && <StepTools onNext={() => setStep("github")} onBack={() => setStep("red")} />}
      {step === "github" && <StepGithub onNext={() => setStep("done")} onBack={() => setStep("tools")} />}
      {step === "done" && me && <StepDone me={me} onOk={() => onOk(me)} onBack={() => setStep("github")} />}
    </AccessShell>
  );
}

function Nav({ onBack, onNext, nextLabel, nextDisabled, nextAria }: {
  onBack?: () => void;
  onNext: () => void;
  nextLabel: string;
  nextDisabled?: boolean;
  nextAria?: string;
}) {
  const { t } = useI18n();
  return (
    <div className="flex items-center gap-2">
      {onBack && (
        <Button type="button" variant="ghost" size="sm" onClick={onBack} aria-label={t("common.back")}>
          ←
        </Button>
      )}
      <Button type="button" variant="outline" size="sm" onClick={onNext} aria-label={t("system.auth.skip")}>
        {t("system.auth.skip")}
      </Button>
      <span className="flex-1" />
      <Button type="button" size="sm" disabled={nextDisabled} onClick={onNext} aria-label={nextAria ?? nextLabel}>
        {nextLabel}
      </Button>
    </div>
  );
}

function StepAdmin({ onDone }: { onDone: (me: Me) => void }) {
  const { t } = useI18n();
  const [user, setUser] = useState("");
  const [p1, setP1] = useState("");
  const [p2, setP2] = useState("");
  const [err, setErr] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    if (p1 !== p2) {
      setErr(t("system.auth.passwordMismatch"));
      return;
    }
    try {
      onDone(await setup(user.trim(), p1));
    } catch (e) {
      setErr(e instanceof Error ? e.message : t("common.error"));
    }
  }

  return (
    <form onSubmit={submit} className="space-y-3">
      <Err msg={err} />
      <label className="field-label">
        {t("system.auth.user")}
        <Input
          value={user}
          onChange={(e) => setUser(e.target.value)}
          placeholder={t("system.auth.userPlaceholder")}
          autoComplete="username"
          className="h-8 font-mono text-[12px]"
        />
      </label>
      <label className="field-label">
        {t("system.auth.password")}
        <Input
          type="password"
          value={p1}
          onChange={(e) => setP1(e.target.value)}
          placeholder={t("system.auth.passwordMinPlaceholder")}
          autoComplete="new-password"
          className="tnum h-8 font-mono text-[12px]"
        />
      </label>
      <label className="field-label">
        {t("system.auth.repeatPassword")}
        <Input
          type="password"
          value={p2}
          onChange={(e) => setP2(e.target.value)}
          placeholder={t("system.auth.repeatPasswordPlaceholder")}
          autoComplete="new-password"
          className="tnum h-8 font-mono text-[12px]"
        />
      </label>
      <Button type="submit" className="mt-1 h-8 w-full justify-between">
        <span>{t("system.auth.createAccountNext")}</span>
        <span aria-hidden className="font-mono text-[9px] tracking-[0.08em] opacity-60">↵</span>
      </Button>
    </form>
  );
}

function StepRed({ onNext, onBack }: { onNext: () => void; onBack: () => void }) {
  const { t } = useI18n();
  const [domain, setDomain] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function save() {
    setErr("");
    const v = domain.trim();
    if (v === "") {
      onNext();
      return;
    }
    setBusy(true);
    try {
      await setSetting("base_domain", v);
      onNext();
    } catch (e) {
      setErr(e instanceof Error ? e.message : t("common.error"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3">
      <Err msg={err} />
      <p className="font-mono text-[11px] leading-relaxed text-muted-foreground">{t("system.auth.redDesc")}</p>
      <label className="field-label">
        {t("system.auth.baseDomain")}
        <Input
          value={domain}
          onChange={(e) => setDomain(e.target.value)}
          placeholder="midominio.com (vacío = solo localhost)"
          autoComplete="off"
          className="h-8 font-mono text-[12px]"
        />
      </label>
      <p className="font-mono text-[10px] leading-relaxed text-muted-foreground">{t("system.auth.redHint")}</p>
      <Nav onBack={onBack} onNext={save} nextLabel={domain.trim() ? t("system.auth.saveNext") : t("system.auth.skipNext")} nextDisabled={busy} />
    </div>
  );
}

function StepTools({ onNext, onBack }: { onNext: () => void; onBack: () => void }) {
  const { t } = useI18n();
  const [checks, setChecks] = useState<DoctorCheck[] | null>(null);
  const [err, setErr] = useState("");
  const [fixKey, setFixKey] = useState<string | null>(null);
  const [fixVal, setFixVal] = useState("");
  const [saving, setSaving] = useState(false);

  async function load() {
    setErr("");
    try {
      setChecks(await fetchDoctor());
    } catch (e) {
      setErr(e instanceof Error ? e.message : t("common.error"));
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function saveFix(key: string) {
    setErr("");
    setSaving(true);
    try {
      await setSetting(key, fixVal.trim());
      setFixKey(null);
      setFixVal("");
      await load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : t("common.error"));
    } finally {
      setSaving(false);
    }
  }

  const fails = (checks ?? []).filter((c) => c.state === "fail").length;
  return (
    <div className="space-y-3">
      <Err msg={err} />
      <p className="font-mono text-[11px] leading-relaxed text-muted-foreground">{t("system.auth.toolsDesc")}</p>
      {checks === null ? (
        <p className="font-mono text-[11px] text-muted-foreground">{t("system.auth.checking")}</p>
      ) : (
        <ul className="max-h-64 space-y-1.5 overflow-auto border border-border/70 p-2">
          {checks.map((c) => (
            <li key={c.name} className="space-y-1 border-b border-border/40 pb-1.5 last:border-0 last:pb-0">
              <p className="flex items-baseline gap-2 font-mono text-[11px]">
                <span className={c.state === "ok" ? "text-ok" : c.state === "warn" ? "text-warn" : "text-bad"}>
                  {c.state === "ok" ? "✓" : c.state === "warn" ? "!" : "✗"}
                </span>
                <span className="min-w-0 flex-1 truncate">{c.name}</span>
                <span className="truncate text-[10px] text-muted-foreground">{c.detail}</span>
              </p>
              {c.hint && <p className="pl-5 font-mono text-[10px] text-muted-foreground">→ {c.hint}</p>}
              {(c.name === "node (node_bin)" || c.name === "python (python_bin)") && c.state !== "ok" && (
                fixKey === c.name ? (
                  <div className="flex items-center gap-1.5 pl-5">
                    <Input
                      value={fixVal}
                      onChange={(e) => setFixVal(e.target.value)}
                      placeholder="C:\\nodejs\\node.exe"
                      className="h-7 font-mono text-[11px]"
                      autoFocus
                    />
                    <Button type="button" size="xs" disabled={fixVal.trim() === "" || saving} onClick={() => saveFix(c.name.includes("node") ? "node_bin" : "python_bin")}>
                      {t("common.save")}
                    </Button>
                  </div>
                ) : (
                  <div className="pl-5">
                    <button
                      type="button"
                      onClick={() => { setFixKey(c.name); setFixVal(""); }}
                      className="font-mono text-[10px] text-signal hover:underline"
                    >
                      {t("system.auth.fixBin")}
                    </button>
                  </div>
                )
              )}
            </li>
          ))}
        </ul>
      )}
      <div className="flex items-center gap-2">
        <Button type="button" variant="outline" size="xs" onClick={load}>
          {t("system.auth.recheck")}
        </Button>
        {fails > 0 && <span className="font-mono text-[10px] text-bad">{t("system.auth.fails", { n: fails })}</span>}
      </div>
      <Nav onBack={onBack} onNext={onNext} nextLabel={t("system.auth.skipNext")} />
      <p className="font-mono text-[9px] text-muted-foreground/70">{t("system.auth.laterNote")}</p>
    </div>
  );
}

function StepGithub({ onNext, onBack }: { onNext: () => void; onBack: () => void }) {
  const { t } = useI18n();
  const [st, setSt] = useState<GithubStatus | null>(null);
  const [understood, setUnderstood] = useState(false);

  useEffect(() => {
    getGithubStatus().then(setSt).catch(() => setSt(null));
  }, []);

  const perms = [
    t("system.auth.ghPermContents"),
    t("system.auth.ghPermMetadata"),
    t("system.auth.ghPermPush"),
    t("system.auth.ghPermWebhook"),
  ];

  return (
    <div className="space-y-3">
      <p className="font-mono text-[11px] leading-relaxed text-muted-foreground">{t("system.auth.ghDesc")}</p>
      <ul className="space-y-1 border border-border/70 p-2">
        {perms.map((p) => (
          <li key={p} className="flex items-baseline gap-2 font-mono text-[11px]">
            <span aria-hidden className="text-signal">+</span>
            <span className="text-muted-foreground">{p}</span>
          </li>
        ))}
      </ul>
      <p className="border border-warn/40 bg-warn/5 px-2 py-1.5 font-mono text-[10px] leading-relaxed text-muted-foreground">
        {t("system.auth.ghScopeWarn")}
      </p>
      {st?.configured ? (
        <p className="font-mono text-[11px] text-ok">{t("system.auth.ghConfigured", { account: st.account ?? "?" })}</p>
      ) : (
        st?.setup_url && (
          <Button type="button" variant="outline" size="sm" className="w-full justify-between" onClick={() => window.open(st.setup_url, "_blank", "noopener")}>
            <span>{t("system.auth.ghConnect")}</span>
            <span aria-hidden>↗</span>
          </Button>
        )
      )}
      <label className="flex cursor-pointer items-start gap-2 border border-border/70 px-2 py-1.5">
        <input
          type="checkbox"
          checked={understood}
          onChange={(e) => setUnderstood(e.target.checked)}
          className="mt-0.5 accent-[var(--signal)]"
          aria-label={t("system.auth.ghOptOut")}
        />
        <span className="font-mono text-[10px] leading-relaxed text-muted-foreground">{t("system.auth.ghOptOut")}</span>
      </label>
      <Nav onBack={onBack} onNext={onNext} nextLabel={t("system.auth.skipNext")} nextDisabled={!understood && !st?.configured} />
    </div>
  );
}

function StepDone({ me, onOk, onBack }: { me: Me; onOk: () => void; onBack: () => void }) {
  const { t } = useI18n();
  const [brand, setBrand] = useState<string | null>(null);
  useEffect(() => {
    getSettings().then((gs) => setBrand(gs.site_name || null)).catch(() => setBrand(null));
  }, []);
  return (
    <div className="space-y-3">
      <p className="font-mono text-[11px] leading-relaxed text-muted-foreground">
        {t("system.auth.doneDesc", { user: me.user })}
      </p>
      <ul className="space-y-1 border border-border/70 p-2 font-mono text-[11px] text-muted-foreground">
        <li>✓ {t("system.auth.doneAdmin", { user: me.user })}</li>
        <li>{brand ? `✓ ${t("system.auth.doneBase", { base: brand })}` : `· ${t("system.auth.doneBaseAuto")}`}</li>
        <li>· {t("system.auth.doneLater")}</li>
      </ul>
      <div className="flex items-center gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={onBack} aria-label={t("common.back")}>
          ←
        </Button>
        <span className="flex-1" />
        <Button type="button" size="sm" onClick={onOk} aria-label={t("system.auth.finish")}>
          {t("system.auth.finish")}
        </Button>
      </div>
    </div>
  );
}
