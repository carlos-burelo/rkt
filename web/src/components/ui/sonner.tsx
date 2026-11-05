import { Toaster as Sonner, type ToasterProps } from "sonner";
import { useTheme } from "../../shared/theme";

// Toaster sigue el tema resuelto (claro/oscuro).
const Toaster = ({ ...props }: ToasterProps) => {
  const { resolved } = useTheme();
  return (
  <Sonner
    theme={resolved}
    className="toaster group"
    toastOptions={{
      classNames: {
        toast: "group toast group-[.toaster]:bg-card group-[.toaster]:text-foreground group-[.toaster]:border-border",
        description: "group-[.toast]:text-muted-foreground",
        actionButton: "group-[.toast]:bg-primary group-[.toast]:text-primary-foreground",
        cancelButton: "group-[.toast]:bg-muted group-[.toast]:text-muted-foreground",
      },
    }}
    {...props}
  />
  );
};

export { Toaster };
