import { useEffect, useState } from "react";

// InstallButton prompt de instalación PWA (Chrome/Edge; null en resto).
export function InstallButton() {
  const [deferred, setDeferred] = useState<Event | null>(null);

  useEffect(() => {
    const h = (e: Event) => {
      e.preventDefault();
      setDeferred(e);
    };
    window.addEventListener("beforeinstallprompt", h);
    return () => window.removeEventListener("beforeinstallprompt", h);
  }, []);

  if (!deferred) return null;
  return (
    <button
      onClick={() => {
        (deferred as unknown as { prompt: () => void }).prompt();
        setDeferred(null);
      }}
      className="block w-full rounded-md bg-primary px-2.5 py-1.5 text-left text-xs font-medium text-primary-foreground hover:bg-primary/90"
    >
      Instalar app
    </button>
  );
}
