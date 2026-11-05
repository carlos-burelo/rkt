import { useEffect, useState } from "react";
import { Plus } from "lucide-react";
import { User } from "../../shared/lib/types";
import { listUsers, createUser, deleteUser } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader, EmptyState, Time } from "../../shared/components/patterns";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { Avatar } from "../../shared/components/ui";
import { roleLabel } from "../../shared/lib/labels";
import {
  Select, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

const ROLE_CLASS: Record<string, string> = {
  admin: "border-signal/35 bg-signal/10 text-signal",
  operator: "border-info/30 bg-info/10 text-info",
  viewer: "border-border text-muted-foreground",
};

export function Users() {
  const fail = useUI((u) => u.fail);
  const me = useAuth((a) => a.who);
  const [users, setUsers] = useState<User[]>([]);
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("viewer");

  async function load() {
    try {
      setUsers(await listUsers());
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
        title="Usuarios"
        description="Solo admin. Viewer lee, operator muta la flota, admin gestiona usuarios y configuración."
        meta={<span className="tnum font-mono text-[10px] text-muted-foreground">{users.length} cuentas</span>}
        actions={
          <FormDialog
            trigger={
              <Button size="sm">
                <Plus className="h-3 w-3" /> nuevo usuario
              </Button>
            }
            title="Nuevo usuario"
            description="lectura lee · operador muta la flota · administrador gestiona todo · contraseña mín. 12"
            submitLabel="crear"
            canSubmit={name.trim() !== "" && password.length >= 12}
            onSubmit={async () => {
              await createUser(name.trim(), password, role);
              setName("");
              setPassword("");
              setRole("viewer");
              await load();
            }}
          >
            <Field label="usuario">
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="usuario"
                className="h-8 font-mono text-[12px]"
                autoFocus
              />
            </Field>
            <Field label="contraseña" hint="mín. 12">
              <Input
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="mínimo 12 caracteres"
                type="password"
                className="tnum h-8 font-mono text-[12px]"
              />
            </Field>
            <Field label="rol">
              <Select value={role} onValueChange={(v: string | null) => setRole(v ?? "viewer")}>
                <SelectTrigger className="h-8 font-mono text-[12px]">
                  <span className="capitalize">{roleLabel(role)}</span>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="viewer">lectura</SelectItem>
                  <SelectItem value="operator">operador</SelectItem>
                  <SelectItem value="admin">administrador</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </FormDialog>
        }
      />
      <div className="border border-border" role="table" aria-label="usuarios">
        <div
          className="hidden grid-cols-[minmax(0,1fr)_7rem_9rem_4rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 sm:grid"
          aria-hidden
        >
          <span className="micro">usuario</span>
          <span className="micro">rol</span>
          <span className="micro text-right">alta</span>
          <span />
        </div>
        {users.map((u) => (
          <div
            key={u.name}
            className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 px-3 py-2.5 last:border-0 sm:grid-cols-[minmax(0,1fr)_7rem_9rem_4rem] sm:gap-3"
          >
            <span className="flex min-w-0 items-center gap-2">
              <Avatar name={u.name} />
              <span className="truncate font-mono text-[12px]">{u.name}</span>
              {me?.user === u.name && (
                <span className="font-mono text-[9px] tracking-[0.08em] text-muted-foreground uppercase">tú</span>
              )}
            </span>
            <span
              className={`tnum inline-flex h-5 w-fit items-center border px-1.5 font-mono text-[10px] tracking-[0.08em] uppercase ${
                ROLE_CLASS[u.role] ?? ROLE_CLASS.viewer
              }`}
            >
              {roleLabel(u.role)}
            </span>
            <span className="tnum hidden text-right font-mono text-[11px] text-muted-foreground sm:block">
              <Time iso={u.created_at} />
            </span>
            <div className="flex justify-end">
              <ConfirmButton
                disabled={me?.user === u.name}
                title={me?.user === u.name ? "no puedes borrarte a ti mismo" : "borrar usuario"}
                confirmLabel="confirmar"
                onConfirm={() => {
                  deleteUser(u.name).then(load).catch(fail);
                }}
              />
            </div>
          </div>
        ))}
        {users.length === 0 && <EmptyState title="Sin usuarios" hint="Crea el primero con «nuevo usuario»." />}
      </div>
    </div>
  );
}
