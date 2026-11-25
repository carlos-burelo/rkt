import { useEffect, useState, useRef } from "react";
import { ArrowLeft, ArrowUpRight, CornerDownLeft } from "lucide-react";
import { TechIcon } from "../../shared/components/TechIcon";
import { setup, me as fetchMe } from "./api";
import { useAuth } from "./store";
import { Me } from "../../shared/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Err } from "../../shared/components/ui";
import { AccessShell } from "./AccessShell";
import { useI18n } from "../../shared/i18n";
import { useUI } from "../../app/ui";
import { fetchDoctor, type DoctorCheck } from "../server/api";
import { getSettings, setSetting, getGithubStatus, type GithubStatus } from "../settings/api";
import { fixHost } from "../routes/api";

type Step = "admin" | "red" | "tools" | "github" | "done";

const STEP_KEY = "rkt.setup.step";
const ME_KEY = "rkt.setup.me";

// El setup abre sesión al crear el admin: un reload (HMR) a mitad del
// wizard te encontraba logueado y la puerta te sacaba a /overview sin
// poder continuar. El progreso sobrevive en sessionStorage (por pestaña)
// y la ruta /setup admite admins con sesión para terminar lo pendiente.
function loadStep(): Step {
  try {
    const s = sessionStorage.getItem(STEP_KEY);
    if (s === "admin" || s === "red" || s === "tools" || s === "github" || s === "done") return s;
  } catch {
    /* sin storage: desde el inicio */
  }
  return "admin";
}

function loadMe(): Me | null {
  try {
    const raw = sessionStorage.getItem(ME_KEY);
    if (raw) return JSON.parse(raw) as Me;
  } catch {
    /* sin storage: sin cuenta */
  }
  return useAuth.getState().who;
}

function clearResume() {
  try {
    sessionStorage.removeItem(STEP_KEY);
    sessionStorage.removeItem(ME_KEY);
  } catch {
    /* sin storage: nada que limpiar */
  }
}

// Setup por pasos: admin (obligatorio) + red/herramientas/GitHub
// omitibles. El doctor corre en el paso 3 con reparación in situ
// (bins). Todo lo omitido sigue disponible en el dashboard.
export function Setup({ onOk }: { onOk: (me: Me) => void }) {
  const { t } = useI18n();
  const [step, setStepState] = useState<Step>(() => {
    const s = loadStep();
    // Sin cuenta (ni creada ni sesión), el wizard empieza en admin.
    if (s !== "admin" && !loadMe()) return "admin";
    // Admin con sesión retomando sin progreso guardado (p. ej. reload
    // antes del fix): la cuenta ya existe, sigue en red.
    if (s === "admin" && useAuth.getState().setupRequired === false) return "red";
    return s;
  });
  const [me, setMeState] = useState<Me | null>(() => loadMe());
  const steps: Step[] = ["admin", "red", "tools", "github", "done"];
  const idx = steps.indexOf(step);

  const setStep = (s: Step) => {
    setStepState(s);
    try {
      sessionStorage.setItem(STEP_KEY, s);
    } catch {
      /* sin storage: se pierde al recargar */
    }
  };
  const setMe = (m: Me | null) => {
    setMeState(m);
    try {
      if (m) sessionStorage.setItem(ME_KEY, JSON.stringify(m));
      else sessionStorage.removeItem(ME_KEY);
    } catch {
      /* sin storage: se pierde al recargar */
    }
  };
  const finish = (m: Me) => {
    clearResume();
    onOk(m);
  };

  return (
    <AccessShell
      title={t("system.auth.setupTitle")}
      description={t("system.auth.setupDesc")}
      wide
      side={
        <ol className="space-y-1" aria-label={t("system.auth.stepsAria")}>
          {steps.map((s, i) => {
            const done = i < idx;
            const active = i === idx;
            return (
              <li
                key={s}
                aria-current={active ? "step" : undefined}
                className={`flex items-center gap-3 border px-3 py-2.5 ${
                  active
                    ? "border-signal/50 bg-signal/[0.07]"
                    : done
                      ? "border-border/60 bg-card/30"
                      : "border-transparent"
                }`}
              >
                <span
                  aria-hidden
                  className={`tnum flex h-6 w-6 shrink-0 items-center justify-center font-mono text-[11px] ${
                    active
                      ? "bg-signal font-bold text-black"
                      : done
                        ? "bg-ok/20 text-ok"
                        : "bg-secondary text-muted-foreground"
                  }`}
                >
                  {done ? "✓" : i + 1}
                </span>
                <span className="min-w-0">
                  <span className={`block text-[13px] ${active ? "font-medium text-foreground" : done ? "text-foreground/80" : "text-muted-foreground"}`}>
                    {t(`system.auth.step.${s}`)}
                  </span>
                  <span className="mt-0.5 block truncate font-mono text-[10px] text-muted-foreground">
                    {t(`system.auth.stepDesc.${s}`)}
                  </span>
                </span>
                {active && <span aria-hidden className="ml-auto h-4 w-px shrink-0 bg-signal" />}
              </li>
            );
          })}
        </ol>
      }
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
              {step === "admin" && (
                <>
                  <br />
                  {t("system.auth.setupFooterTwo")}
                </>
              )}
          </p>
        </div>
      }
    >
      {step === "admin" && <StepAdmin onDone={(m) => { setMe(m); setStep("red"); }} />}
      {step === "red" && <StepRed onNext={() => setStep("tools")} onBack={() => setStep("admin")} />}
      {step === "tools" && <StepTools onNext={() => setStep("github")} onBack={() => setStep("red")} />}
      {step === "github" && <StepGithub onNext={() => setStep("done")} onBack={() => setStep("tools")} />}
      {step === "done" && me && <StepDone me={me} onOk={() => finish(me)} onBack={() => setStep("github")} />}
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
          <ArrowLeft className="h-3 w-3" />
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
  const fail = useUI((u) => u.fail);
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
      // La cuenta se creó pero el paso se perdió (reload entre POST y
      // avance): retoma con la sesión que el setup ya abrió.
      const msg = e instanceof Error ? e.message : t("common.error");
      if (msg.includes("ya completado")) {
        try {
          onDone(await fetchMe());
          return;
        } catch {
          /* cae al toast */
        }
      }
      fail(e);
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
        <CornerDownLeft aria-hidden className="h-3 w-3 opacity-60" />
      </Button>
    </form>
  );
}

function StepRed({ onNext, onBack }: { onNext: () => void; onBack: () => void }) {
  const { t } = useI18n();
  const fail = useUI((u) => u.fail);
  const [domain, setDomain] = useState("");
  const [busy, setBusy] = useState(false);

  async function save() {
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
      fail(e);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3">
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

// Nombre legible para el doctor: el backend habla de bins y settings,
// el usuario ve herramientas. Sin tocar el backend (sus tests usan esos
// nombres).
function prettyCheckName(name: string, t: (k: string) => string): string {
  if (name.startsWith("node ")) return t("system.auth.checkNode");
  if (name.startsWith("python ")) return t("system.auth.checkPython");
  if (name.startsWith("port_pool")) return t("system.auth.checkPorts");
  if (name.startsWith("symlink ")) return t("system.auth.checkSymlink");
  return name;
}

function StepTools({ onNext, onBack }: { onNext: () => void; onBack: () => void }) {
  const { t } = useI18n();
  const fail = useUI((u) => u.fail);
  const [checks, setChecks] = useState<DoctorCheck[] | null>(null);
  const [fixKey, setFixKey] = useState<string | null>(null);
  const [fixVal, setFixVal] = useState("");
  const [saving, setSaving] = useState(false);

  async function load() {
    try {
      setChecks(await fetchDoctor());
    } catch (e) {
      fail(e);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function saveFix(key: string) {
    setSaving(true);
    try {
      await setSetting(key, fixVal.trim());
      setFixKey(null);
      setFixVal("");
      await load();
    } catch (e) {
      fail(e);
    } finally {
      setSaving(false);
    }
  }

  const fails = (checks ?? []).filter((c) => c.state === "fail").length;
  return (
    <div className="space-y-3">
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
                <span className="min-w-0 flex-1 truncate">{prettyCheckName(c.name, t)}</span>
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
  const [polling, setPolling] = useState(false);
  const pollTimer = useRef<number | null>(null);

  const checkStatus = () => {
    return getGithubStatus().then((s) => {
      setSt(s);
      return s;
    }).catch(() => {
      setSt(null);
      return null;
    });
  };

  useEffect(() => {
    checkStatus();
  }, []);

  useEffect(() => {
    if (!polling) {
      if (pollTimer.current) clearInterval(pollTimer.current);
      return;
    }
    pollTimer.current = window.setInterval(async () => {
      const s = await checkStatus();
      if (s?.configured) {
        setPolling(false);
      }
    }, 2500);

    return () => {
      if (pollTimer.current) clearInterval(pollTimer.current);
    };
  }, [polling]);

  const onConnectClick = () => {
    if (st?.setup_url) {
      window.open(st.setup_url, "_blank", "noopener");
      setPolling(true);
    }
  };

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
      {st?.error && !st.configured && (
        <p className="border border-warn/40 bg-warn/5 px-2 py-1.5 font-mono text-[10px] leading-relaxed text-warn">
          {st.error}
          {st.fix && <><br />{st.fix}</>}
        </p>
      )}
      {st?.configured ? (
        <div className="border border-ok/40 bg-ok/5 px-3 py-2 font-mono text-[11px] text-ok">
          {t("system.auth.ghSuccess", { account: st.account ?? "?" })}
        </div>
      ) : (
        st?.setup_url && (
          <div className="space-y-1.5">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="w-full justify-between"
              onClick={onConnectClick}
            >
              <span className="flex items-center gap-2">
                <TechIcon kind="github" size={16} />
                {polling ? t("system.auth.ghWaiting") : t("system.auth.ghConnect")}
              </span>
              <ArrowUpRight aria-hidden className="h-3 w-3" />
            </Button>
            {polling && (
              <div className="flex items-center justify-between px-1">
                <span className="font-mono text-[10px] text-signal animate-pulse">{t("system.auth.ghWaiting")}</span>
                <button
                  type="button"
                  onClick={() => checkStatus()}
                  className="font-mono text-[10px] text-muted-foreground hover:underline"
                >
                  {t("system.auth.ghCheckNow")}
                </button>
              </div>
            )}
          </div>
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
      <Nav onBack={onBack} onNext={onNext} nextLabel={st?.configured ? t("common.next") : t("system.auth.skipNext")} nextDisabled={!understood && !st?.configured} />
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
          <ArrowLeft className="h-3 w-3" />
        </Button>
        <span className="flex-1" />
        <Button type="button" size="sm" onClick={onOk} aria-label={t("system.auth.finish")}>
          {t("system.auth.finish")}
        </Button>
      </div>
    </div>
  );
}
