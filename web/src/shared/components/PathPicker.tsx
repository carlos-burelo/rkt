import { useEffect, useMemo, useState } from "react";
import { ArrowUp, Check, File, Folder, Loader2, Search } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { listFS, FsEntry, FsListing } from "../../features/fs/api";
import { useUI } from "../../app/ui";

const SEP = /[\\/]/;

// Padre "barato" sin depender del backend: sirve para abrir el picker
// en el directorio del valor actual mientras llega el listing.
function dirOf(value: string): string {
  const trimmed = value.trim();
  if (trimmed === "") return "";
  const idx = Math.max(trimmed.lastIndexOf("\\"), trimmed.lastIndexOf("/"));
  if (idx <= 0) return trimmed;
  return trimmed.slice(0, idx);
}

// PathPicker: navegador de rutas del host para no escribir paths a mano.
// mode=dir elige carpeta; mode=file elige archivo (o carpeta también).
export function PathPicker({
  open,
  setOpen,
  value,
  mode,
  onPick,
  title,
}: {
  open: boolean;
  setOpen: (b: boolean) => void;
  value: string;
  mode: "dir" | "file";
  onPick: (path: string) => void;
  title?: string;
}) {
  const fail = useUI((u) => u.fail);
  const [path, setPath] = useState("");
  const [listing, setListing] = useState<FsListing | null>(null);
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setQ("");
    const start = value.trim() === "" ? "" : mode === "dir" ? value.trim() : dirOf(value.trim());
    load(start);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  function load(p: string) {
    setBusy(true);
    listFS(p)
      .then((l) => {
        setListing(l);
        setPath(l.path);
      })
      .catch((e) => {
        fail(e);
        setListing(null);
      })
      .finally(() => setBusy(false));
  }

  const rows = useMemo(() => {
    const all = listing?.entries ?? [];
    const f = q.trim().toLowerCase();
    return f === "" ? all : all.filter((e) => e.name.toLowerCase().includes(f));
  }, [listing, q]);

  function pick(e: FsEntry) {
    if (e.dir) {
      load(e.path);
      return;
    }
    if (mode === "file") {
      onPick(e.path);
      setOpen(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o: boolean) => {
        if (!o) setOpen(false);
      }}
    >
      <DialogContent className="gap-3 sm:max-w-xl">
        <DialogHeader className="pr-8">
          <div className="flex items-center gap-2">
            <span aria-hidden className="h-1 w-4 bg-signal" />
            <span className="micro">{mode === "dir" ? "elegir carpeta" : "elegir archivo"}</span>
          </div>
          <DialogTitle className="mt-1 text-[16px] tracking-[-0.02em]">
            {title ?? "Rutas del host"}
          </DialogTitle>
        </DialogHeader>

        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="icon-sm"
            onClick={() => load(listing?.parent ?? "")}
            disabled={busy || !listing || listing.parent === ""}
            aria-label="subir un nivel"
            title="subir un nivel"
          >
            <ArrowUp className="h-3.5 w-3.5" />
          </Button>
          <Input
            value={path}
            onChange={(e) => setPath(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") load(path);
            }}
            placeholder="C:\ruta\exacta"
            className="h-7 min-w-0 flex-1 font-mono text-[11px]"
            aria-label="ruta"
          />
          <Button variant="outline" size="sm" onClick={() => load(path)} disabled={busy}>
            ir
          </Button>
        </div>

        <div className="relative">
          <Search className="absolute top-1/2 left-2 h-3 w-3 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="filtrar por nombre…"
            className="h-7 pl-7 font-mono text-[11px]"
            aria-label="filtrar"
          />
        </div>

        <div className="h-64 overflow-y-auto border border-border" role="listbox" aria-label="contenido del directorio">
          {busy && (
            <p className="flex items-center gap-2 px-3 py-2 font-mono text-[10px] text-muted-foreground">
              <Loader2 className="h-3 w-3 animate-spin" /> leyendo…
            </p>
          )}
          {!busy && rows.length === 0 && (
            <p className="px-3 py-2 font-mono text-[10px] text-muted-foreground">sin coincidencias</p>
          )}
          {!busy &&
            rows.map((e) => (
              <button
                key={e.path}
                type="button"
                role="option"
                aria-selected={false}
                onClick={() => pick(e)}
                className="flex w-full items-center gap-2 border-b border-border/60 px-3 py-1.5 text-left last:border-0 hover:bg-accent/40"
              >
                {e.dir ? (
                  <Folder className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                ) : (
                  <File className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60" />
                )}
                <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{e.name}</span>
                {!e.dir && mode === "file" && (
                  <span className="font-mono text-[9px] text-muted-foreground/60 uppercase">elegir</span>
                )}
              </button>
            ))}
        </div>

        <div className="flex flex-wrap items-center gap-2 border-t border-border pt-3">
          <p className="min-w-0 flex-1 truncate font-mono text-[9px] text-muted-foreground" title={path || listing?.path}>
            {path || listing?.path || "raíces del disco"}
          </p>
          <Button
            size="sm"
            disabled={!listing || listing.path === ""}
            onClick={() => {
              const target = listing?.path ?? "";
              if (target === "") return;
              onPick(target);
              setOpen(false);
            }}
          >
            <Check className="h-3 w-3" /> usar esta carpeta
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// PathInput: input de ruta con botón de picker. Un solo camino para
// cada campo de path (comando = archivo, dir = carpeta).
export function PathInput({
  value,
  onChange,
  mode,
  placeholder,
  disabled,
  className,
  ariaLabel,
}: {
  value: string;
  onChange: (v: string) => void;
  mode: "dir" | "file";
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  ariaLabel?: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div className="flex items-center gap-1">
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        disabled={disabled}
        aria-label={ariaLabel}
        className={className ?? "h-7 min-w-0 flex-1 font-mono text-[11px]"}
      />
      <Button
        type="button"
        variant="outline"
        size="icon-sm"
        disabled={disabled}
        onClick={() => setOpen(true)}
        aria-label={mode === "dir" ? "elegir carpeta" : "elegir archivo"}
        title={mode === "dir" ? "elegir carpeta" : "elegir archivo"}
      >
        {mode === "dir" ? <Folder className="h-3.5 w-3.5" /> : <File className="h-3.5 w-3.5" />}
      </Button>
      <PathPicker
        open={open}
        setOpen={setOpen}
        value={value}
        mode={mode}
        onPick={onChange}
      />
    </div>
  );
}
