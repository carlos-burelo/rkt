import { useState } from "react";
import { Root, Backdrop, Popup, Title, Close } from "@base_ui/react/dialog";
import { createApp } from "./api";

// CreateDialog alta de app (repo/URL o manual) o ir a Domains.
export function CreateDialog({
  open,
  setOpen,
  onCreated,
  goDomains,
  fail,
}: {
  open: boolean;
  setOpen: (b: boolean) => void;
  onCreated: (name: string) => void;
  goDomains: () => void;
  fail: (e: unknown) => void;
}) {
  const [repo, setRepo] = useState("");
  const [name, setName] = useState("");
  const [command, setCommand] = useState("");
  const [busy, setBusy] = useState(false);

  return (
    <Root open={open} onOpenChange={setOpen}>
      <Backdrop className="fixed inset-0 bg-black/70" />
      <Popup className="fixed left-1/2 top-24 w-full max-w-md -translate-x-1/2 rounded-lg border border-zinc-700 bg-zinc-950 p-6">
        <div className="mb-4 flex items-center gap-2">
          <Title className="text-base font-medium">Create New</Title>
          <span className="flex-1" />
          <Close className="rounded border border-zinc-800 px-2 py-0.5 text-xs text-zinc-400 hover:text-zinc-100">
            cerrar
          </Close>
        </div>
        <div className="space-y-2">
          <input
            value={repo}
            onChange={(e) => setRepo(e.target.value)}
            placeholder="repo git https o dir local (vacío = manual)"
            className="w-full rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none"
          />
          <div className="flex gap-2">
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="nombre [a-z0-9-]"
              className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none"
            />
            <input
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              placeholder="binario absoluto"
              className="flex-1 rounded border border-zinc-700 bg-zinc-900 px-2 py-1.5 font-mono text-xs outline-none"
            />
          </div>
          <button
            disabled={busy}
            onClick={() => {
              setBusy(true);
              createApp({ repo: repo.trim() || undefined, name: name.trim() || undefined, command: command.trim() || undefined })
                .then((app) => {
                  setBusy(false);
                  setOpen(false);
                  setRepo("");
                  setName("");
                  setCommand("");
                  onCreated(app);
                })
                .catch((e) => {
                  setBusy(false);
                  fail(e);
                });
            }}
            className="w-full rounded bg-zinc-100 px-2 py-1.5 text-sm font-medium text-zinc-900 hover:bg-white disabled:opacity-50"
          >
            {busy ? "creando…" : "Crear app"}
          </button>
          <button
            onClick={() => {
              setOpen(false);
              goDomains();
            }}
            className="w-full rounded border border-zinc-700 px-2 py-1.5 text-xs hover:bg-zinc-800"
          >
            Conectar dominio →
          </button>
        </div>
      </Popup>
    </Root>
  );
}
