import { useState } from "react";
import { History, KeyRound, Plus, Power } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import { roleLabel } from "../../shared/lib/labels";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import {
  changePassword, createUser, listSessions, revokeSession,
} from "./api";
import type { UserSession } from "../../shared/lib/types";
import { useUsers } from "./users.store";

// Diálogos con estado de formulario local: cada instancia es
// independiente (no va al store global para no fugar entre filas).

export function PasswordDialog({ name, self, onSaved }: { name: string; self: boolean; onSaved: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [cur, setCur] = useState("");
  const [next, setNext] = useState("");
  const [repeat, setRepeat] = useState("");
  return (
    <FormDialog
      open={open}
      onOpenChange={setOpen}
      trigger={
        <Button variant={self ? "outline" : "ghost"} size="xs" title={t("system.users.passwordTitle")}>
          <KeyRound className="h-3 w-3" /> {self ? t("system.users.passwordSelf") : ""}
        </Button>
      }
      title={t("system.users.passwordTitleFor", { name })}
      description={t("system.users.passwordDesc")}
      submitLabel={t("common.save")}
      canSubmit={next.length >= 12 && next === repeat && (!self || cur !== "")}
      onSubmit={async () => {
        const token = await changePassword(self ? "me" : name, cur, next);
        if (token) {
          onSaved();
        }
        setCur("");
        setNext("");
        setRepeat("");
        toast.success(t("system.users.passwordSaved"));
      }}
    >
      {self && (
        <Field label={t("system.users.currentPassword")}>
          <Input type="password" value={cur} onChange={(e) => setCur(e.target.value)} className="h-8 font-mono text-[12px]" autoFocus />
        </Field>
      )}
      <Field label={t("system.users.newPassword")} hint={t("system.users.passwordHint")}>
        <Input type="password" value={next} onChange={(e) => setNext(e.target.value)} className="h-8 font-mono text-[12px]" />
      </Field>
      <Field label={t("system.users.repeatPassword")}>
        <Input type="password" value={repeat} onChange={(e) => setRepeat(e.target.value)} className="h-8 font-mono text-[12px]" />
      </Field>
    </FormDialog>
  );
}

export function SessionsDialog({ name, onChanged }: { name: string; onChanged: () => void }) {
  const { t } = useI18n();
  const fail = useUI((u) => u.fail);
  const [open, setOpen] = useState(false);
  const [sessions, setSessions] = useState<UserSession[]>([]);
  const [current, setCurrent] = useState("");
  const load = () => {
    listSessions(name)
      .then((r) => {
        setSessions(r.sessions);
        setCurrent(r.current);
      })
      .catch(fail);
  };
  return (
    <FormDialog
      open={open}
      onOpenChange={(o) => {
        setOpen(o);
        if (o) load();
      }}
      trigger={
        <Button variant="ghost" size="xs" title={t("system.users.sessions")}>
          <History className="h-3 w-3" />
        </Button>
      }
      title={t("system.users.sessionsTitle", { name })}
      description={t("system.users.sessionsDesc")}
      submitLabel={t("common.close")}
      onSubmit={() => setOpen(false)}
    >
      <div className="max-h-64 overflow-auto border border-border">
        {sessions.map((s) => (
          <div key={s.id} className="flex items-center gap-2 border-b border-border/60 px-2 py-1.5 last:border-0">
            <span className="min-w-0 flex-1">
              <span className="block font-mono text-[10px]">{s.created_at.replace("T", " ").slice(0, 19)}</span>
              <span className="block font-mono text-[9px] text-muted-foreground">{s.expires_at.slice(0, 10)}</span>
            </span>
            {s.id === current && <span className="font-mono text-[9px] text-ok uppercase">{t("system.users.current")}</span>}
            <ConfirmButton
              label={t("system.users.revoke")}
              confirmLabel={t("common.confirm")}
              onConfirm={() => revokeSession(name, s.id).then(() => { load(); onChanged(); }).catch(fail)}
            />
          </div>
        ))}
        {sessions.length === 0 && <p className="px-2 py-4 font-mono text-[10px] text-muted-foreground">{t("system.users.noSessions")}</p>}
      </div>
    </FormDialog>
  );
}

export function NewUserDialog({ onCreated }: { onCreated: () => void }) {
  const { t } = useI18n();
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("viewer");
  return (
    <FormDialog
      trigger={
        <Button size="sm">
          <Plus className="h-3 w-3" /> {t("system.users.new")}
        </Button>
      }
      title={t("system.users.newTitle")}
      description={t("system.users.newDesc")}
      submitLabel={t("common.create")}
      canSubmit={name.trim() !== "" && password.length >= 12}
      onSubmit={async () => {
        await createUser(name.trim(), password, role);
        setName("");
        setPassword("");
        setRole("viewer");
        onCreated();
      }}
    >
      <Field label={t("system.users.user")}>
        <Input value={name} onChange={(e) => setName(e.target.value)} className="h-8 font-mono text-[12px]" autoFocus />
      </Field>
      <Field label={t("system.users.password")} hint={t("system.users.passwordHint")}>
        <Input value={password} onChange={(e) => setPassword(e.target.value)} type="password" className="tnum h-8 font-mono text-[12px]" />
      </Field>
      <Field label={t("system.users.role")}>
        <Select value={role} onValueChange={(v: string | null) => setRole(v ?? "viewer")}>
          <SelectTrigger className="h-8 font-mono text-[12px]">
            <span className="capitalize">{roleLabel(role)}</span>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="viewer">{roleLabel("viewer")}</SelectItem>
            <SelectItem value="operator">{roleLabel("operator")}</SelectItem>
            <SelectItem value="admin">{roleLabel("admin")}</SelectItem>
          </SelectContent>
        </Select>
      </Field>
    </FormDialog>
  );
}

export function UserPowerButton({ name, disabled, self }: { name: string; disabled?: boolean; self: boolean }) {
  const { t } = useI18n();
  const toggle = useUsers((s) => s.toggle);
  return (
    <Button
      variant="ghost"
      size="xs"
      disabled={self}
      title={disabled ? t("system.users.enable") : t("system.users.disable")}
      onClick={() => toggle(name, !disabled)}
    >
      <Power className={`h-3 w-3 ${disabled ? "text-muted-foreground" : "text-ok"}`} />
    </Button>
  );
}
