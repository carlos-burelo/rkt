import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

// Confirmación en dos pasos para acciones irreversibles de fila.
// Primer click arma, segundo ejecuta; se desarma solo a los 4 s.
export function ConfirmButton({
  onConfirm,
  label = "borrar",
  confirmLabel = "confirmar",
  title,
  className,
  disabled,
  size = "xs",
}: {
  onConfirm: () => void;
  label?: string;
  confirmLabel?: string;
  title?: string;
  className?: string;
  disabled?: boolean;
  size?: "xs" | "sm";
}) {
  const [armed, setArmed] = useState(false);

  useEffect(() => {
    if (!armed) return;
    const t = setTimeout(() => setArmed(false), 4000);
    return () => clearTimeout(t);
  }, [armed]);

  return (
    <Button
      variant={armed ? "destructive" : "ghost"}
      size={size}
      disabled={disabled}
      aria-live="polite"
      title={armed ? "click otra vez para confirmar" : title}
      className={cn(!armed && "text-muted-foreground hover:bg-bad/10 hover:text-bad", armed && "border-bad/60 bg-bad/20", className)}
      onClick={() => {
        if (!armed) {
          setArmed(true);
          return;
        }
        setArmed(false);
        onConfirm();
      }}
    >
      {armed ? confirmLabel : label}
    </Button>
  );
}
