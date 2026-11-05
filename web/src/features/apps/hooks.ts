import { useCallback, useEffect, useState } from "react";
import { getEnvKeys } from "./api";
import { useUI } from "../../app/ui";

// useEnv claves de una app; el valor se revela por clave.
export function useEnv(app: string): { keys: string[]; reload: () => void } {
  const fail = useUI((u) => u.fail);
  const online = useUI((u) => u.online);
  const [keys, setKeys] = useState<string[]>([]);
  const [gen, setGen] = useState(0);
  const reload = useCallback(() => setGen((g) => g + 1), []);
  useEffect(() => {
    if (!app || !online) return;
    getEnvKeys(app)
      .then(setKeys)
      .catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [app, online, gen]);
  return { keys, reload };
}

// useEnvCount solo el conteo (checklist sin traer valores).
export function useEnvCount(app: string): number {
  const { keys } = useEnv(app);
  return keys.length;
}
