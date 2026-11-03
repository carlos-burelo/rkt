import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { Me } from "../shared/lib/types";
import { logout, me, authStatus } from "../features/auth/api";
import { getSettings } from "../features/settings/api";

// Auth global: sesión, setup inicial, marca. Un hogar.
interface Auth {
  who: Me | null;
  authed: boolean;
  isAdmin: boolean;
  canWrite: boolean;
  brand: string;
  setupRequired: boolean | null;
  setWho: (m: Me | null) => void;
  doLogout: () => void;
}

const Ctx = createContext<Auth | null>(null);

export function useAuth(): Auth {
  const a = useContext(Ctx);
  if (!a) throw new Error("useAuth fuera de AuthProvider");
  return a;
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [who, setWho] = useState<Me | null>(null);
  const [brand, setBrand] = useState("rkt");
  const [setupRequired, setSetupRequired] = useState<boolean | null>(null);

  useEffect(() => {
    authStatus()
      .then((s) => {
        setSetupRequired(s.setup_required);
        if (s.setup_required) return;
        return me()
          .then((m) => {
            setWho(m);
            getSettings()
              .then((gs) => {
                if (gs.site_name) setBrand(gs.site_name);
              })
              .catch(() => {});
          })
          .catch(() => setWho(null));
      })
      .catch(() => {
        setSetupRequired(false);
        setWho(null);
      });
  }, []);

  const doLogout = useCallback(() => {
    logout().finally(() => setWho(null));
  }, []);

  const authed = who !== null;
  return (
    <Ctx.Provider
      value={{
        who, authed,
        isAdmin: who?.role === "admin",
        canWrite: who?.role === "admin" || who?.role === "operator",
        brand, setupRequired, setWho, doLogout,
      }}
    >
      {children}
    </Ctx.Provider>
  );
}
