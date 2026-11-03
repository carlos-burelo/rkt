import { Toaster as Sonner, type ToasterProps } from "sonner";

// Toaster fijo dark (sin next-themes: SPA Vite, no Next).
const Toaster = ({ ...props }: ToasterProps) => (
  <Sonner
    theme="dark"
    className="toaster group"
    toastOptions={{
      classNames: {
        toast: "group toast group-[.toaster]:bg-zinc-900 group-[.toaster]:text-zinc-100 group-[.toaster]:border-zinc-800",
        description: "group-[.toast]:text-zinc-400",
        actionButton: "group-[.toast]:bg-zinc-100 group-[.toast]:text-zinc-900",
        cancelButton: "group-[.toast]:bg-zinc-800 group-[.toast]:text-zinc-300",
      },
    }}
    {...props}
  />
);

export { Toaster };
