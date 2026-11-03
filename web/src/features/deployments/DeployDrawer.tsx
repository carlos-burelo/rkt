import { Root, Backdrop, Popup, Title, Close } from "@base_ui/react/dialog";
import { Deployment } from "../../shared/lib/types";
import { rel, dur } from "../../shared/lib/time";
import { Dot } from "../../shared/components/ui";

export function DeployDrawer({
  dep,
  buildLog,
  canWrite,
  onAction,
  onClose,
}: {
  dep: Deployment | null;
  buildLog: string;
  canWrite: boolean;
  onAction: (app: string, a: "restart" | "deploy" | "rollback") => void;
  onClose: () => void;
}) {
  return (
    <Root
      open={dep !== null}
      onOpenChange={(open: boolean) => {
        if (!open) onClose();
      }}
    >
      <Backdrop className="fixed inset-0 bg-black/70" />
      <Popup className="fixed right-0 top-0 flex h-full w-full max-w-xl flex-col border-l border-zinc-800 bg-zinc-950">
        {dep && (
          <div className="flex h-full flex-col space-y-4 overflow-hidden p-6">
            <div className="flex items-center gap-2">
              <Dot status={dep.status} />
              <Title className="font-mono text-base">
                {dep.app} <span className="text-zinc-500">#{dep.id}</span>
              </Title>
              <span className="flex-1" />
              <Close className="rounded border border-zinc-800 px-2 py-0.5 text-xs text-zinc-400 hover:text-zinc-100">
                cerrar
              </Close>
            </div>
            <dl className="grid grid-cols-2 gap-1.5 font-mono text-xs">
              <dt className="text-zinc-500">status</dt><dd>{dep.status}</dd>
              <dt className="text-zinc-500">rev</dt><dd className="truncate">{dep.rev}</dd>
              <dt className="text-zinc-500">inicio</dt><dd>{rel(dep.started_at)}</dd>
              <dt className="text-zinc-500">duración</dt><dd>{dur(dep.started_at, dep.finished_at)}</dd>
            </dl>
            {canWrite && (
              <div className="flex gap-2">
                {(["restart", "deploy", "rollback"] as const).map((a) => (
                  <button key={a} onClick={() => onAction(dep.app, a)}
                    className="rounded-md border border-zinc-700 px-2.5 py-1 text-xs hover:bg-zinc-800">
                    {a} {dep.app}
                  </button>
                ))}
              </div>
            )}
            <p className="text-xs uppercase tracking-wider text-zinc-500">build log</p>
            <pre className="min-h-0 flex-1 overflow-auto rounded-lg border border-zinc-800 bg-black p-3 font-mono text-xs leading-relaxed text-zinc-300">
              {buildLog}
            </pre>
          </div>
        )}
      </Popup>
    </Root>
  );
}
