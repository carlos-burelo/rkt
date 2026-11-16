import { useParams } from "react-router";
import { DeployDetail } from "./DeployDetail";

// Página dedicada de despliegue (estilo Vercel): foco total en el log,
// sin sheet lateral. Ruta /deployments/:id.
export function DeployPage() {
  const { id = "" } = useParams();
  const num = Number(id);
  return <DeployDetail id={Number.isFinite(num) ? num : -1} />;
}
