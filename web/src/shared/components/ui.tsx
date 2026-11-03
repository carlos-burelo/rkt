import { Avatar as AvatarPrimitive, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";

export function Dot({ status }: { status: string }) {
  const ok = status === "ok" || status === "running";
  const run = status === "running";
  return (
    <span
      className={`inline-block h-2 w-2 rounded-full ${ok ? "bg-emerald-400" : "bg-red-400"} ${run ? "animate-pulse" : ""}`}
      title={status}
    />
  );
}

export function Err({ msg }: { msg: string }) {
  if (!msg) return null;
  return <p className="mb-4 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">{msg}</p>;
}

export function Avatar({ name }: { name: string }) {
  const ch = (name.trim()[0] ?? "?").toUpperCase();
  return (
    <AvatarPrimitive className="h-6 w-6 text-[11px]">
      <AvatarFallback>{ch}</AvatarFallback>
    </AvatarPrimitive>
  );
}

