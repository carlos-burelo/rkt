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
      className="block w-full border border-signal/40 bg-signal/10 px-2 py-1.5 text-left font-mono text-[10px] tracking-[0.08em] text-signal uppercase hover:bg-signal/15"
    >
      Instalar app
    </button>
  );
}
