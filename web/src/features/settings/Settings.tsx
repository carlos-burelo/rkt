import { setSetting } from "./api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent } from "@/components/ui/card";

export function Settings({
  settings,
  canWrite,
  refresh,
  fail,
}: {
  settings: Record<string, string>;
  canWrite: boolean;
  refresh: () => void;
  fail: (e: unknown) => void;
}) {
  return (
    <Card>
      <CardContent className="space-y-1 pt-6">
        {Object.entries(settings).map(([k, v]) => (
          <div key={k} className="flex items-center gap-2 py-1.5">
            <span className="w-40 font-mono text-xs text-muted-foreground">{k}</span>
            <Input
              defaultValue={v}
              id={`set-${k}`}
              disabled={!canWrite}
              className="flex-1 font-mono text-xs"
            />
            {canWrite && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  const el = document.getElementById(`set-${k}`) as HTMLInputElement;
                  setSetting(k, el.value).then(refresh).catch(fail);
                }}
              >
                guardar
              </Button>
            )}
          </div>
        ))}
      </CardContent>
    </Card>
  );
}
