import { useEffect, useState } from "react";
import { User } from "../../shared/lib/types";
import { listUsers, createUser, deleteUser } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader, EmptyState, Section, Time } from "../../shared/components/patterns";
import { Avatar, Dot } from "../../shared/components/ui";
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
      />
      <div className="border border-border" role="table" aria-label="usuarios">
        <div
          className="hidden grid-cols-[1.5rem_minmax(0,1fr)_7rem_9rem_4rem] items-center gap-3 border-b border-border bg-secondary/25 px-3 py-1.5 sm:grid"
          aria-hidden
        >
          <span className="micro">est</span>
          <span className="micro">usuario</span>
          <span className="micro">rol</span>
          <span className="micro text-right">alta</span>
          <span />
        </div>
        {users.map((u) => (
          <div
            key={u.name}
            className="grid grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-2 border-b border-border/70 bg-card/35 px-3 py-2.5 last:border-0 sm:grid-cols-[1.5rem_minmax(0,1fr)_7rem_9rem_4rem] sm:gap-3"
          >
            <Dot status={me?.user === u.name ? "running" : "stopped"} />
            <span className="flex min-w-0 items-center gap-2">
              <Avatar name={u.name} />
              <span className="truncate font-mono text-[11px]">{u.name}</span>
              {me?.user === u.name && (
                <span className="font-mono text-[8px] tracking-[0.08em] text-muted-foreground uppercase">tú</span>
              )}
            </span>
            <span
              className={`tnum inline-flex h-5 w-fit items-center border px-1.5 font-mono text-[9px] tracking-[0.08em] uppercase ${
                ROLE_CLASS[u.role] ?? ROLE_CLASS.viewer
              }`}
            >
              {roleLabel(u.role)}
            </span>
            <span className="tnum hidden text-right font-mono text-[10px] text-muted-foreground sm:block">
              <Time iso={u.created_at} />
            </span>
            <div className="flex justify-end">
              <Button
                variant="ghost"
                size="xs"
                className="text-muted-foreground hover:text-bad"
                disabled={me?.user === u.name}
                title={me?.user === u.name ? "no puedes borrarte a ti mismo" : "borrar"}
                onClick={() => deleteUser(u.name).then(load).catch(fail)}
              >
                borrar
              </Button>
            </div>
          </div>
        ))}
        {users.length === 0 && <EmptyState title="Sin usuarios" hint="Crea el primero abajo." />}
      </div>

      <Section title="Nuevo usuario" description="lectura lee · operador muta · administrador gestiona · password mín 12">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
          <label className="field-label w-32 shrink-0">
            usuario
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="usuario" className="h-7 font-mono text-[11px]" />
          </label>
          <label className="field-label min-w-0 flex-1">
            password
            <Input
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="mín 12 caracteres"
              type="password"
              className="tnum h-7 font-mono text-[11px]"
            />
          </label>
          <label className="field-label w-32 shrink-0">
            rol
            <Select value={role} onValueChange={(v: string | null) => setRole(v ?? "viewer")}>
              <SelectTrigger className="h-7 font-mono text-[11px]">
                <span className="capitalize">{roleLabel(role)}</span>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="viewer">lectura</SelectItem>
                <SelectItem value="operator">operador</SelectItem>
                <SelectItem value="admin">administrador</SelectItem>
              </SelectContent>
            </Select>
          </label>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              createUser(name.trim(), password, role)
                .then(() => {
                  setName("");
                  setPassword("");
                  setRole("viewer");
                  load();
                })
                .catch(fail);
            }}
          >
            crear
          </Button>
        </div>
      </Section>
    </div>
  );
}
