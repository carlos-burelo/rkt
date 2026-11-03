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
  return <p className="mb-4 rounded-md border border-red-900 bg-red-950/50 px-3 py-2 text-xs text-red-300">{msg}</p>;
}

export function Note({ msg }: { msg: string }) {
  if (!msg) return null;
  return <p className="mb-4 rounded-md border border-emerald-900 bg-emerald-950/50 px-3 py-2 text-xs text-emerald-300">{msg}</p>;
}
