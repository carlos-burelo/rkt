import { Status } from "./types";

// publicUrl arma la URL real que sirve el proxy. En dev el listener es
// :8080, así que un dominio sin puerto (80) daría conexión rechazada.
export function publicUrl(host: string, st?: Status | null): string {
  const clean = host.trim().replace(/^https?:\/\//, "");
  if (st?.https_port) {
    return `https://${clean}${st.https_port === "443" ? "" : `:${st.https_port}`}`;
  }
  const port = st?.http_port || "80";
  return `http://${clean}${port === "80" ? "" : `:${port}`}`;
}

// publicHost muestra host:puerto cuando el proxy no está en el puerto estándar.
export function publicHost(host: string, st?: Status | null): string {
  if (st?.https_port) {
    return st.https_port === "443" ? host : `${host}:${st.https_port}`;
  }
  const port = st?.http_port || "80";
  return port === "80" ? host : `${host}:${port}`;
}
