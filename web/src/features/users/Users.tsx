import { useEffect, useState } from "react";
import { History, KeyRound, Plus, Power } from "lucide-react";
import { toast } from "sonner";
import { AuditEntry, User, UserSession } from "../../shared/lib/types";
import {
  listUsers, createUser, deleteUser, setUser, changePassword, listSessions, revokeSession, listAudit,
} from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader, EmptyState, Time, Section } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { Avatar } from "../../shared/components/ui";
import { roleLabel } from "../../shared/lib/labels";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";

const ROLE_CLASS: Record<string, string> = {
  admin: "border-signal/35 bg-signal/10 text-signal",
  operator: "border-info/30 bg-info/10 text-info",
  viewer: "border-border text-muted-foreground",
};

function PasswordDialog({ name, self, onSaved }: { name: string; self: boolean; onSaved: () => void }) {
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

function SessionsDialog({ name, onChanged }: { name: string; onChanged: () => void }) {
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
  useEffect(() => {
    if (open) load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, name]);
  return (
    <FormDialog
      open={open}
      onOpenChange={setOpen}
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

export function Users() {
  const { t, tn } = useI18n();
  const fail = useUI((u) => u.fail);
  const me = useAuth((a) => a.who);
  const [users, setUsers] = useState<User[]>([]);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("viewer");

  async function load() {
    try {
      setUsers(await listUsers());
      listAudit().then(setAudit).catch(() => {});
    } catch (e) {
      fail(e);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("nav.users")}
        description={t("system.users.description")}
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{tn("system.users.accounts", users.length)}</span>}
        actions={
          <div className="flex items-center gap-2">
            {me && <PasswordDialog name={me.user} self onSaved={() => {}} />}
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
                await load();
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
          </div>
        }
      />
      <div className="border border-border" role="table" aria-label={t("nav.users")}>
        <div
          className="hidden grid-cols-[minmax(0,1fr)_7rem_9rem_8rem_9rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 sm:grid"
          aria-hidden
        >
          <span className="micro">{t("system.users.user")}</span>
          <span className="micro">{t("system.users.role")}</span>
          <span className="micro">{t("system.users.createdAt")}</span>
          <span className="micro text-right">{t("system.users.lastLogin")}</span>
          <span />
        </div>
        {users.map((u) => (
          <div
            key={u.name}
            className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 px-3 py-2.5 last:border-0 sm:grid-cols-[minmax(0,1fr)_7rem_9rem_8rem_9rem] sm:gap-3"
          >
            <span className="flex min-w-0 items-center gap-2">
              <Avatar name={u.name} />
              <span className={`truncate font-mono text-[12px] ${u.disabled ? "text-muted-foreground line-through" : ""}`}>{u.name}</span>
              {me?.user === u.name && (
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
              <PasswordDialog name={u.name} self={false} onSaved={load} />
              <SessionsDialog name={u.name} onChanged={load} />
              <Button
                variant="ghost"
                size="xs"
                disabled={me?.user === u.name}
                title={u.disabled ? t("system.users.enable") : t("system.users.disable")}
                onClick={() => setUser(u.name, { disabled: !u.disabled }).then(load).catch(fail)}
              >
                <Power className={`h-3 w-3 ${u.disabled ? "text-muted-foreground" : "text-ok"}`} />
              </Button>
              <ConfirmButton
                disabled={me?.user === u.name}
                title={me?.user === u.name ? t("system.users.cannotDeleteSelf") : t("system.users.deleteTitle")}
                confirmLabel={t("common.confirm")}
                onConfirm={() => deleteUser(u.name).then(load).catch(fail)}
              />
            </div>
          </div>
        ))}
        {users.length === 0 && <EmptyState title={t("system.users.emptyTitle")} hint={t("system.users.emptyHint")} />}
      </div>

      <Section title={t("system.users.auditTitle")} description={t("system.users.auditDesc")}>
        <div className="border border-border">
          {audit.slice(0, 20).map((a) => (
            <div key={a.id} className="grid grid-cols-[8rem_minmax(0,1fr)_auto] items-baseline gap-2 border-b border-border/60 px-3 py-1.5 last:border-0">
              <span className="tnum font-mono text-[9px] text-muted-foreground">{a.ts.replace("T", " ").slice(0, 19)}</span>
              <span className="min-w-0 truncate font-mono text-[10px]">
                <b className="text-foreground/80">{a.actor}</b> · {a.action} · {a.target}
              </span>
              <span className="font-mono text-[9px] text-muted-foreground">{a.ip}</span>
            </div>
          ))}
          {audit.length === 0 && <p className="px-3 py-3 font-mono text-[10px] text-muted-foreground">{t("system.users.noAudit")}</p>}
        </div>
      </Section>
    </div>
  );
}
