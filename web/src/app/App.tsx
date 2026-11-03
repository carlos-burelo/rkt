import { useEffect } from "react";
import { useAuth } from "../features/auth/store";
import { AppRouter } from "./router";

// App delgado: boot de sesión + router real. Estado en zustand
// (features/auth/store, app/store). Nada de lógica aquí.
export function App() {
  const boot = useAuth((a) => a.boot);
  useEffect(() => {
    boot();
  }, [boot]);
  return <AppRouter />;
}
