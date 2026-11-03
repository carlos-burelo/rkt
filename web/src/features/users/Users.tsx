import { useEffect, useState } from "react";
import { User } from "../../shared/lib/types";
import { listUsers, createUser, deleteUser } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { PageHeader, EmptyState, Section, Time } from "../../shared/components/patterns";
import { Avatar, Dot } from "../../shared/components/ui";
import {
  Select, SelectValue, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";
import { useUI } from "../../app/ui";
import { useAuth } from "../auth/store";

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
    <div className="space-y-3">
      <PageHeader title="Users" description="Solo admin. Roles: viewer lee, operator muta, admin gestiona." />
      <div className="overflow-hidden rounded-lg border" role="table" aria-label="usuarios">
        <div className="tnum hidden grid-cols-[2rem_1fr_7rem_10rem_5rem] items-center gap-3 border-b bg-muted/40 px-4 py-1.5 font-mono text-[10px] uppercase tracking-wider text-muted-foreground sm:grid" aria-hidden>
          <span>est</span>
          <span>usuario</span>
          <span>rol</span>
          <span className="text-right">alta</span>
          <span className="text-right">acción</span>
        </div>
        {users.map((u) => (
          <div key={u.name} className="flex items-center gap-3 border-b bg-card/40 px-4 py-2.5 last:border-0 sm:grid sm:grid-cols-[2rem_1fr_7rem_10rem_5rem]">
            <Dot status={me?.user === u.name ? "running" : "stopped"} />
            <span className="flex min-w-0 flex-1 items-center gap-2">
              <Avatar name={u.name} />
              <span className="truncate font-mono text-xs">{u.name}</span>
              {me?.user === u.name && (
                <Badge variant="outline" className="font-mono text-[11px]">tú</Badge>
              )}
            </span>
            <Badge variant="secondary" className="tnum font-mono text-[11px] font-normal">{u.role}</Badge>
            <span className="tnum hidden text-right font-mono text-[11px] text-muted-foreground sm:block"><Time iso={u.created_at} /></span>
            <span className="flex-1 sm:hidden" />
            <span className="text-right">
              <Button variant="ghost" size="sm" className="text-xs text-muted-foreground hover:text-red-300"
                disabled={me?.user === u.name}
                title={me?.user === u.name ? "no puedes borrarte a ti mismo" : "borrar"}
                onClick={() => deleteUser(u.name).then(load).catch(fail)}>
                borrar
              </Button>
            </span>
          </div>
        ))}
        {users.length === 0 && <EmptyState title="Sin usuarios" hint="Crea el primero abajo." />}
      </div>
      <Section title="Nuevo usuario" description="Viewer lee · operator muta · admin gestiona. Mín 12 caracteres.">
        <div className="flex flex-col gap-2 sm:flex-row">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="usuario"
            className="w-32 font-mono text-xs" />
          <Input value={password} onChange={(e) => setPassword(e.target.value)} placeholder="password (mín 12)" type="password"
            className="tnum flex-1 font-mono text-xs" />
          <Select value={role} onValueChange={(v: string | null) => setRole(v ?? "viewer")}>
            <SelectTrigger className="w-32 font-mono text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="viewer">viewer</SelectItem>
              <SelectItem value="operator">operator</SelectItem>
              <SelectItem value="admin">admin</SelectItem>
            </SelectContent>
          </Select>
          <Button
            variant="outline" size="sm"
            onClick={() => {
              createUser(name.trim(), password, role)
                .then(() => {
                  setName("");
                  setPassword("");
                  setRole("viewer");
                  load();
                }).catch(fail);
            }}
          >
            crear
          </Button>
        </div>
      </Section>
    </div>
  );
}
