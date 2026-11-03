import { api } from "../../shared/lib/http";

export interface FsEntry {
  name: string;
  path: string;
  dir: boolean;
}

export interface FsListing {
  path: string;
  parent: string;
  entries: FsEntry[];
  truncated: boolean;
}

// listFS navega el disco del host (solo operator+). Sin path = raíces.
export function listFS(path: string): Promise<FsListing> {
  const q = path === "" ? "" : `?path=${encodeURIComponent(path)}`;
  return api<FsListing>(`/api/v1/fs${q}`);
}
