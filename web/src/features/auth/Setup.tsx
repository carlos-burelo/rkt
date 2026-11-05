import { useState } from "react";
import { setup } from "./api";
import { Me } from "../../shared/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Err } from "../../shared/components/ui";
import { AccessShell } from "./AccessShell";
import { useI18n } from "../../shared/i18n";

// Setup primer arranque: crea el admin (one-shot, el backend lo
// permite solo sin usuarios). Sin comandos.
export function Setup({ onOk }: { onOk: (me: Me) => void }) {
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
      const me = await setup(user.trim(), p1);
      onOk(me);
    } catch (e) {
      setErr(e instanceof Error ? e.message : t("common.error"));
    }
  }

  return (
    <AccessShell
      title={t("system.auth.setupTitle")}
      description={t("system.auth.setupDesc")}
      footer={
        <p className="font-mono text-[9px] leading-relaxed text-muted-foreground/70">
          {t("system.auth.setupFooterOne")}
          <br />
          {t("system.auth.setupFooterTwo")}
        </p>
      }
    >
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
          <span>{t("system.auth.createAccount")}</span>
          <span aria-hidden className="font-mono text-[9px] tracking-[0.08em] opacity-60">↵</span>
        </Button>
      </form>
    </AccessShell>
  );
}
