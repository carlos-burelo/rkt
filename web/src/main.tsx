import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import { App } from "./app/App";

// SW con scope relativo: la SPA vive en /dashboard/.
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("sw.js", { scope: "./" }).catch(() => {
      // sin SW (dev/http): la app funciona igual, sin offline
    });
  });
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
