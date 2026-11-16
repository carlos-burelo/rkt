import { useState } from "react";
import { CornerDownLeft } from "lucide-react";
import { login } from "./api";
import { Me } from "../../shared/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AccessShell } from "./AccessShell";
import { useI18n } from "../../shared/i18n";
import { useUI } from "../../app/ui";

export function Login({ onOk }: { onOk: (me: Me) => void }) {
  const { t } = useI18n();
  const fail = useUI((u) => u.fail);
  const [user, setUser] = useState("");
  const [pass, setPass] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    try {
      const me = await login(user.trim(), pass);
      onOk(me);
    } catch (e) {
      fail(e);
    }
  }

  return (
    <AccessShell
      title={t("system.auth.loginTitle")}
      description={t("system.auth.loginDesc")}
      footer={
        <p className="font-mono text-[9px] leading-relaxed text-muted-foreground/70">
          {t("system.auth.noAccountBefore")} <span className="text-foreground/80">rkt users add</span>
          <br />
          {t("system.auth.permissions", {
            viewer: t("role.viewer"),
            operator: t("role.operator"),
            admin: t("role.admin"),
          })}
        </p>
      }
    >
      <form onSubmit={submit} className="space-y-3">
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
            value={pass}
            onChange={(e) => setPass(e.target.value)}
            placeholder={t("system.auth.passwordPlaceholder")}
            autoComplete="current-password"
            className="tnum h-8 font-mono text-[12px]"
          />
        </label>
        <Button type="submit" className="mt-1 h-8 w-full justify-between">
          <span>{t("system.auth.signIn")}</span>
          <CornerDownLeft aria-hidden className="h-3 w-3 opacity-60" />
        </Button>
      </form>
    </AccessShell>
  );
}
