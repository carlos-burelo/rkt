import { useCallback, useEffect, useState } from "react";
import { useUI } from "../../app/ui";
import { getEnvIndex } from "./api";

// useGlobalEnv solo claves: los valores se piden al revelar.
export function useGlobalEnv(): { keys: string[]; appKeys: Record<string, string[]>; reload: () => void } {
  const fail = useUI((u) => u.fail);
  const online = useUI((u) => u.online);
  const [keys, setKeys] = useState<string[]>([]);
  const [appKeys, setAppKeys] = useState<Record<string, string[]>>({});
  const [gen, setGen] = useState(0);
  const reload = useCallback(() => setGen((g) => g + 1), []);
  useEffect(() => {
    if (!online) return;
    getEnvIndex()
      .then((idx) => {
        setKeys(idx.global);
        setAppKeys(idx.apps);
      })
      .catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [online, gen]);
  return { keys, appKeys, reload };
}
