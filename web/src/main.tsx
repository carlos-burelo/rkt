import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import { App } from "./app/App";

// SW con scope absoluto desde la raíz: un deep link (p. ej.
// /app/x/logs) no debe resolver assets/manifest relativos a esa ruta.
if ("serviceWorker" in navigator) {
  const base = import.meta.env.BASE_URL;
  // Si ya había un SW controlando, un cambio de versión recarga una vez
  // para no quedarse sirviendo el bundle viejo desde caché.
  if (navigator.serviceWorker.controller) {
    let reloading = false;
    navigator.serviceWorker.addEventListener("controllerchange", () => {
      if (reloading) return;
      reloading = true;
      window.location.reload();
    });
  }
  window.addEventListener("load", () => {
    navigator.serviceWorker.register(`${base}sw.js`, { scope: base }).catch(() => {
      // sin SW (dev/http): la app funciona igual, sin offline
    });
  });
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
