import { useEffect } from "react";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import { useUsers } from "./users.store";
import { NewUserDialog, PasswordDialog } from "./dialogs";
import { UserRow } from "./UserRow";
import { AuditList } from "./AuditList";

// Usuarios: layout + header. Listas en el store, filas y diálogos aparte.
export function Users() {
  const { t, tn } = useI18n();
  const me = useAuth((a) => a.who);
  const users = useUsers((s) => s.users);

  useEffect(() => {
    useUsers.getState().load();
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
            <NewUserDialog onCreated={() => useUsers.getState().load()} />
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
          <UserRow key={u.name} u={u} />
        ))}
        {users.length === 0 && <EmptyState title={t("system.users.emptyTitle")} hint={t("system.users.emptyHint")} />}
      </div>
      <AuditList />
    </div>
  );
}
