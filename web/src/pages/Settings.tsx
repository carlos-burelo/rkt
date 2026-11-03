import { setSetting } from "../api";

export function Settings({
  settings,
  canWrite,
  refresh,
  fail,
}: {
  settings: Record<string, string>;
  canWrite: boolean;
  refresh: () => void;
  fail: (e: unknown) => void;
}) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-4">
      {Object.entries(settings).map(([k, v]) => (
        <div key={k} className="flex items-center gap-2 py-1.5">
          <span className="w-40 font-mono text-xs text-zinc-300">{k}</span>
          <input
            defaultValue={v}
            id={`set-${k}`}
            disabled={!canWrite}
            className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1 font-mono text-xs outline-none disabled:text-zinc-500"
          />
          {canWrite && (
            <button
              onClick={() => {
                const el = document.getElementById(`set-${k}`) as HTMLInputElement;
                setSetting(k, el.value).then(refresh).catch(fail);
              }}
              className="rounded border border-zinc-700 px-2 py-1 text-xs hover:bg-zinc-800"
            >
              guardar
            </button>
          )}
        </div>
      ))}
    </div>
  );
}
