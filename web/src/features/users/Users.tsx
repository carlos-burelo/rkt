import { useState } from "react";
import { User } from "../../shared/lib/types";
import { createUser, deleteUser } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Select, SelectValue, SelectTrigger, SelectContent, SelectItem,
} from "@/components/ui/select";

export function Users({
  users,
  refresh,
  fail,
}: {
  users: User[];
  refresh: () => void;
  fail: (e: unknown) => void;
}) {
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("viewer");

  return (
    <div className="space-y-3">
      <div className="overflow-hidden rounded-lg border border-zinc-800">
        {users.map((u) => (
          <div key={u.name} className="flex items-center gap-3 border-b border-zinc-800 bg-zinc-900/40 px-4 py-2.5 last:border-0">
            <span className="font-mono text-xs">{u.name}</span>
            <Badge variant="secondary" className="font-mono text-[11px]">{u.role}</Badge>
            <span className="flex-1" />
            <span className="font-mono text-[11px] text-zinc-600">{u.created_at}</span>
            <Button variant="ghost" size="sm" className="text-xs text-zinc-600 hover:text-red-300"
              onClick={() => deleteUser(u.name).then(refresh).catch(fail)}>
              borrar
            </Button>
          </div>
        ))}
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
                refresh();
              }).catch(fail);
          }}
        >
          crear
        </Button>
      </div>
    </div>
  );
}
