import { useCallback, useEffect, useState } from "react";
import { getEnv } from "./api";
import { useUI } from "../../app/ui";

// useEnv vars de una app con recarga (operator; el backend manda 403 si no).
export function useEnv(app: string): { env: Record<string, string>; reload: () => void } {
  const fail = useUI((u) => u.fail);
  const online = useUI((u) => u.online);
  const [env, setEnvMap] = useState<Record<string, string>>({});
  const [gen, setGen] = useState(0);
  const reload = useCallback(() => setGen((g) => g + 1), []);
  useEffect(() => {
    if (!app || !online) return;
    getEnv(app).then(setEnvMap).catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [app, online, gen]);
  return { env, reload };
}
