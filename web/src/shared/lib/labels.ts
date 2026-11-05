import { t } from "../i18n";

// Etiquetas visibles en un solo hogar. La API habla inglés (running,
// failed, ok); la UI traduce con el diccionario activo.
function label(status: string, prefix: string): string {
  const key = `${prefix}.${status}`;
  const value = t(key);
  return value === key ? status : value;
}

export function stateLabel(status: string): string {
  return label(status, "state");
}

// Una salida es masculina y "running" significa en curso, no en marcha.
export function deployStateLabel(status: string): string {
  const key = `deploy.${status}`;
  const value = t(key);
  return value === key ? stateLabel(status) : value;
}

export function roleLabel(role: string): string {
  return label(role, "role");
}
