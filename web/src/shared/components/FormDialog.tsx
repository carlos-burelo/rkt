import { useState } from "react";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { useI18n } from "../i18n";

// FormDialog: única forma de crear/editar en la UI. Nada de formularios
// visibles en la página: un botón abre el diálogo, se envía y se cierra.
export function FormDialog({
  trigger,
  title,
  description,
  submitLabel,
  canSubmit = true,
  onSubmit,
  children,
  className,
  open: openProp,
  onOpenChange,
}: {
  trigger: React.ReactElement;
  title: string;
  description?: string;
  submitLabel?: string;
  canSubmit?: boolean;
  onSubmit: () => Promise<void> | void;
  children: React.ReactNode;
  className?: string;
  /** controlado: permite abrir el diálogo desde otro botón de la página */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}) {
  const { t } = useI18n();
  const [openState, setOpenState] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const open = openProp ?? openState;

  const change = (o: boolean) => {
    if (openProp === undefined) setOpenState(o);
    onOpenChange?.(o);
    if (!o) setErr("");
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit || busy) return;
    setBusy(true);
    setErr("");
    try {
      await onSubmit();
      change(false);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={change}>
      <DialogTrigger render={trigger} />
      <DialogContent className={className ?? "sm:max-w-md"}>
        <DialogHeader>
          <div className="flex items-center gap-2 pr-8">
            <span aria-hidden className="h-1 w-4 bg-signal" />
            <span className="micro">rkt</span>
          </div>
          <DialogTitle className="text-[16px] tracking-[-0.02em]">{title}</DialogTitle>
          {description && (
            <DialogDescription className="font-mono text-[10px] leading-relaxed">{description}</DialogDescription>
          )}
        </DialogHeader>
        <form onSubmit={submit} className="space-y-3">
          {children}
          {err && (
            <p className="border border-bad/30 bg-bad/5 px-2 py-1.5 font-mono text-[10px] text-bad">{err}</p>
          )}
          <DialogFooter className="pt-1">
            <DialogClose render={<Button type="button" variant="outline" size="sm" />}>{t("shared.form.cancel")}</DialogClose>
            <Button type="submit" size="sm" disabled={!canSubmit || busy}>
              {busy ? t("shared.form.saving") : (submitLabel ?? t("shared.form.submit"))}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// Field agrupa label + control con el mismo estilo en todos los diálogos.
export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <label className="field-label">
      <span className="flex items-baseline justify-between gap-2">
        <span>{label}</span>
        {hint && <span className="normal-case tracking-normal text-muted-foreground/55">{hint}</span>}
      </span>
      {children}
    </label>
  );
}
