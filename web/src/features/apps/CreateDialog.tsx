import { useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
    <Dialog
      open={open}
      onOpenChange={(o: boolean) => {
        if (!o) setOpen(false);
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create New</DialogTitle>
        </DialogHeader>
        <div className="space-y-2">
          <Input
            value={repo}
            onChange={(e) => setRepo(e.target.value)}
            placeholder="repo git https o dir local (vacío = manual)"
          />
          <div className="flex gap-2">
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="nombre [a-z0-9-]"
            />
            <Input
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              placeholder="binario absoluto"
            />
          </div>
          <Button
            disabled={busy}
            className="w-full"
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
          >
            {busy ? "creando…" : "Crear app"}
          </Button>
          <Button
            variant="outline"
            className="w-full"
            onClick={() => {
              setOpen(false);
              goDomains();
            }}
          >
            Conectar dominio →
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
