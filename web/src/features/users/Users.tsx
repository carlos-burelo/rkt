import { useEffect, useState } from "react";
import { User } from "../../shared/lib/types";
import { listUsers, createUser, deleteUser } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { PageHeader, EmptyState } from "../../shared/components/patterns";
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
      <div className="overflow-hidden rounded-lg border">
        {users.map((u) => (
          <div key={u.name} className="flex items-center gap-3 border-b bg-card/40 px-4 py-2.5 last:border-0">
            <span className="font-mono text-xs">{u.name}</span>
            {me?.user === u.name && (
              <Badge variant="outline" className="font-mono text-[11px]">tú</Badge>
            )}
            <Badge variant="secondary" className="font-mono text-[11px]">{u.role}</Badge>
            <span className="flex-1" />
            <span className="font-mono text-[11px] text-muted-foreground">{u.created_at}</span>
            <Button variant="ghost" size="sm" className="text-xs text-muted-foreground hover:text-red-300"
              disabled={me?.user === u.name}
              title={me?.user === u.name ? "no puedes borrarte a ti mismo" : "borrar"}
              onClick={() => deleteUser(u.name).then(load).catch(fail)}>
              borrar
            </Button>
          </div>
        ))}
        {users.length === 0 && <EmptyState title="Sin usuarios" hint="Crea el primero abajo." />}
      </div>
      <div className="flex gap-2">
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="usuario"
          className="w-32 font-mono text-xs" />
        <Input value={password} onChange={(e) => setPassword(e.target.value)} placeholder="password (mín 12)" type="password"
          className="flex-1 font-mono text-xs" />
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
    </div>
  );
}
