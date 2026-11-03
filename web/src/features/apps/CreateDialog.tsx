import { ArrowRight } from "lucide-react";
import { useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
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
      <DialogContent className="gap-4">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <span aria-hidden className="h-1 w-4 bg-signal" />
            <span className="micro">alta de unidad</span>
          </div>
          <DialogTitle className="mt-1 text-[18px] tracking-[-0.02em]">Nueva app</DialogTitle>
          <DialogDescription className="font-mono text-[10px] leading-relaxed">
            repo git, dir local o manual. El nombre queda como identidad de la unidad en el manifiesto.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <label className="field-label">
            repo o directorio
            <Input
              value={repo}
              onChange={(e) => setRepo(e.target.value)}
              placeholder="https://github.com/… o C:\ruta (vacío = manual)"
              className="font-mono text-[11px]"
            />
          </label>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label className="field-label">
              nombre
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="[a-z0-9-]"
                className="font-mono text-[11px]"
              />
            </label>
            <label className="field-label">
              comando
              <Input
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                placeholder="binario absoluto"
                className="font-mono text-[11px]"
              />
            </label>
          </div>
          <Button
            disabled={busy}
            className="w-full justify-between"
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
            <span>{busy ? "creando…" : "Crear app"}</span>
            <span aria-hidden className="font-mono text-[9px] tracking-[0.08em] opacity-60">↵</span>
          </Button>
          <Button
            variant="ghost"
            className="w-full justify-between"
            onClick={() => {
              setOpen(false);
              goDomains();
            }}
          >
            <span>Conectar dominio</span>
            <ArrowRight className="h-3.5 w-3.5" />
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
