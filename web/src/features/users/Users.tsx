import { useState } from "react";
import { User } from "../../shared/lib/types";
import { createUser, deleteUser } from "./api";

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
            <span className="rounded-full bg-zinc-800 px-2 py-0.5 font-mono text-[11px] text-zinc-300">{u.role}</span>
            <span className="flex-1" />
            <span className="font-mono text-[11px] text-zinc-600">{u.created_at}</span>
            <button onClick={() => deleteUser(u.name).then(refresh).catch(fail)}
              className="text-xs text-zinc-600 hover:text-red-300">
              borrar
            </button>
          </div>
        ))}
      </div>
      <div className="flex gap-2">
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="usuario"
          className="w-32 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
        <input value={password} onChange={(e) => setPassword(e.target.value)} placeholder="password (mín 12)" type="password"
          className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none" />
        <select value={role} onChange={(e) => setRole(e.target.value)}
          className="rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none">
          <option value="viewer">viewer</option>
          <option value="operator">operator</option>
          <option value="admin">admin</option>
        </select>
        <button
          onClick={() => {
            createUser(name.trim(), password, role)
              .then(() => {
                setName("");
                setPassword("");
                setRole("viewer");
                refresh();
              }).catch(fail);
          }}
          className="rounded border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800"
        >
          crear
        </button>
      </div>
    </div>
  );
}
