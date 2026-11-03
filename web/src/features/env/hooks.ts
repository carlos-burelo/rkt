import { useCallback, useEffect, useState } from "react";
import { useUI } from "../../app/ui";
import { getGlobalEnv } from "./api";

// useGlobalEnv lee las variables compartidas por la flota.
export function useGlobalEnv(): { env: Record<string, string>; reload: () => void } {
  const fail = useUI((u) => u.fail);
  const online = useUI((u) => u.online);
  const [env, setEnvMap] = useState<Record<string, string>>({});
  const [gen, setGen] = useState(0);
  const reload = useCallback(() => setGen((g) => g + 1), []);
  useEffect(() => {
    if (!online) return;
    getGlobalEnv().then(setEnvMap).catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [online, gen]);
  return { env, reload };
}
