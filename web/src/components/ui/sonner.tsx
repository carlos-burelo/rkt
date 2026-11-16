import { Toaster as Sonner, type ToasterProps } from "sonner";
import { useTheme } from "../../shared/theme";

// Toaster con el lenguaje del sistema: esquinas rectas, borde fino,
// mono, sombra dura, acción outline. Sin redondeados ni pastillas.
const Toaster = ({ ...props }: ToasterProps) => {
  const { resolved } = useTheme();
  return (
  <Sonner
    theme={resolved}
    className="toaster group"
    toastOptions={{
      classNames: {
        toast: "group toast rounded-none border-border bg-popover font-mono text-foreground shadow-[4px_4px_0_0_rgba(0,0,0,0.4)]",
        title: "group-[.toast]:font-mono group-[.toast]:text-[11px] group-[.toast]:text-foreground",
        description: "group-[.toast]:font-mono group-[.toast]:text-[10px] group-[.toast]:text-muted-foreground",
        actionButton: "group-[.toast]:h-6 group-[.toast]:rounded-none group-[.toast]:border group-[.toast]:border-input group-[.toast]:bg-transparent group-[.toast]:px-2 group-[.toast]:font-mono group-[.toast]:text-[10px] group-[.toast]:uppercase group-[.toast]:tracking-[0.08em] group-[.toast]:text-muted-foreground group-[.toast]:shadow-none hover:group-[.toast]:border-foreground hover:group-[.toast]:text-foreground",
        cancelButton: "group-[.toast]:rounded-none group-[.toast]:bg-secondary group-[.toast]:font-mono group-[.toast]:text-muted-foreground group-[.toast]:shadow-none",
        closeButton: "group-[.toast]:rounded-none group-[.toast]:border group-[.toast]:border-border group-[.toast]:bg-transparent group-[.toast]:text-muted-foreground",
      },
    }}
    {...props}
  />
  );
};

export { Toaster };
