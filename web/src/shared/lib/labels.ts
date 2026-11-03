// Etiquetas visibles en un solo hogar. La API habla inglés (running,
// failed, ok); la UI habla español. Nada de mezclar en cada componente.
const STATE_ES: Record<string, string> = {
  ok: "correcto",
  running: "en marcha",
  degraded: "degradado",
  warning: "degradado",
  warn: "degradado",
  failed: "fallida",
  bad: "fallida",
  stopped: "detenida",
  pending: "pendiente",
};

export function stateLabel(status: string): string {
  return STATE_ES[status] ?? status;
}

const ROLE_ES: Record<string, string> = {
  viewer: "lectura",
  operator: "operador",
  admin: "administrador",
};

export function roleLabel(role: string): string {
  return ROLE_ES[role] ?? role;
}
