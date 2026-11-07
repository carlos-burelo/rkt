import { Power } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Avatar } from "../../shared/components/ui";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { Time } from "../../shared/components/patterns";
import { roleLabel } from "../../shared/lib/labels";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import type { User } from "../../shared/lib/types";
import { useUsers } from "./users.store";
import { PasswordDialog, SessionsDialog } from "./dialogs";

const ROLE_CLASS: Record<string, string> = {
  admin: "border-signal/35 bg-signal/10 text-signal",
  operator: "border-info/30 bg-info/10 text-info",
  viewer: "border-border text-muted-foreground",
};

// Fila de usuario: lee su fila + acciones finas del store (no todo).
export function UserRow({ u }: { u: User }) {
  const { t } = useI18n();
  const me = useAuth((a) => a.who);
  const remove = useUsers((s) => s.remove);
  const toggle = useUsers((s) => s.toggle);
  const refresh = useUsers((s) => s.load);
  const self = me?.user === u.name;
  return (
    <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 px-3 py-2.5 last:border-0 sm:grid-cols-[minmax(0,1fr)_7rem_9rem_8rem_9rem] sm:gap-3">
      <span className="flex min-w-0 items-center gap-2">
        <Avatar name={u.name} />
        <span className={`truncate font-mono text-[12px] ${u.disabled ? "text-muted-foreground line-through" : ""}`}>{u.name}</span>
        {self && (
          <span className="font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase">{t("system.users.you")}</span>
        )}
        {u.disabled && <span className="font-mono text-[9px] text-warn uppercase">{t("system.users.disabled")}</span>}
      </span>
      <span className={`tnum inline-flex h-5 w-fit items-center border px-1.5 font-mono text-[10px] tracking-[0.08em] uppercase ${ROLE_CLASS[u.role] ?? ROLE_CLASS.viewer}`}>
        {roleLabel(u.role)}
      </span>
      <span className="tnum hidden text-right font-mono text-[11px] text-muted-foreground sm:block">
        <Time iso={u.created_at} />
      </span>
      <span className="tnum hidden text-right font-mono text-[11px] text-muted-foreground sm:block">
        {u.last_login ? <Time iso={u.last_login} /> : "—"}
      </span>
      <div className="flex justify-end gap-0.5">
        <PasswordDialog name={u.name} self={false} onSaved={refresh} />
        <SessionsDialog name={u.name} onChanged={refresh} />
        <Button
          variant="ghost"
          size="xs"
          disabled={self}
          title={u.disabled ? t("system.users.enable") : t("system.users.disable")}
          onClick={() => toggle(u.name, !u.disabled)}
        >
          <Power className={`h-3 w-3 ${u.disabled ? "text-muted-foreground" : "text-ok"}`} />
        </Button>
        <ConfirmButton
          disabled={self}
          title={self ? t("system.users.cannotDeleteSelf") : t("system.users.deleteTitle")}
          confirmLabel={t("common.confirm")}
          onConfirm={() => remove(u.name)}
        />
      </div>
    </div>
  );
}
