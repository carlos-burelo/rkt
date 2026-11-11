import { api } from "../../shared/lib/http";
import type { WafBlock } from "../../shared/lib/types";

export interface WafState {
  deny_paths: string[];
  deny_methods: string[];
  blocked: WafBlock[];
}

export async function fetchWaf(): Promise<WafState> {
  return api<WafState>("/api/v1/waf");
}

export async function putWaf(patch: { deny_paths?: string[]; deny_methods?: string[] }): Promise<WafState> {
  return api<WafState>("/api/v1/waf", { method: "PUT", body: JSON.stringify(patch) });
}
