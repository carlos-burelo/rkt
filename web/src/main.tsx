import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import { App } from "./app/App";

// SW con scope absoluto desde la raíz: un deep link (p. ej.
// /app/x/logs) no debe resolver assets/manifest relativos a esa ruta.
// SW solo en build: en dev no existe /sw.js y el fallback HTML ensucia consola.
if ("serviceWorker" in navigator && import.meta.env.PROD) {
  const base = import.meta.env.BASE_URL;
  // Solo recarga si esta pestaña ya estaba controlada: la primera
  // instalación (clientsClaim) no debe refrescar en medio del login.
  const hadController = !!navigator.serviceWorker.controller;
  let reloading = false;
  navigator.serviceWorker.addEventListener("controllerchange", () => {
    if (!hadController || reloading) return;
    reloading = true;
    window.location.reload();
  });
  window.addEventListener("load", () => {
    navigator.serviceWorker
      .register(`${base}sw.js`, { scope: base, updateViaCache: "none" })
      .then((reg) => {
        // buscar versión nueva al volver a la pestaña y cada minuto
        const update = () => reg.update().catch(() => {});
        document.addEventListener("visibilitychange", () => {
          if (!document.hidden) update();
        });
        window.setInterval(update, 60_000);
      })
      .catch(() => {
        // sin SW (dev/http): la app funciona igual, sin offline
      });
  });
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
