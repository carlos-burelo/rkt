// Estado y tono de tareas cron (canónico; lo usan fila e historial).
export type CronState = "ok" | "warn" | "bad" | "running" | "stopped" | "idle";

export function cronStatusOf(job: { running: boolean; last?: { status: string } | null }): string {
  if (job.running) return "running";
  if (!job.last) return "idle";
  return job.last.status;
}

export function cronTone(status: string): Exclude<CronState, "idle"> {
  switch (status) {
    case "ok": return "ok";
    case "running": return "running";
    case "failed": return "bad";
    case "timeout": return "warn";
    default: return "stopped";
  }
}
