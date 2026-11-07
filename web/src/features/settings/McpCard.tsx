import { useEffect, useState } from "react";
import { Check, Copy, KeyRound, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Section } from "../../shared/components/patterns";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import { createToken, listTokens, revokeToken, ApiToken } from "./api";

// McpCard genera API keys para conectar agentes al MCP remoto.
export function McpCard() {
  const { t } = useI18n();
  const isAdmin = useAuth((a) => a.isAdmin);
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [name, setName] = useState("mcp-agent");
  const [created, setCreated] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);
  const endpoint = `${window.location.origin}/mcp`;

  async function load() {
    try {
      setTokens(await listTokens());
    } catch (e) {
      setErr(e instanceof Error ? e.message : t("common.error"));
    }
  }
  useEffect(() => {
    if (isAdmin) load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAdmin]);

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const ta = document.createElement("textarea");
      ta.value = text;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      ta.remove();
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  async function create() {
    setBusy(true);
    setErr("");
    setCreated("");
    try {
      const tok = await createToken(name.trim() || "mcp-agent", "admin", 24 * 90);
      setCreated(tok);
      await load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : t("common.error"));
    } finally {
      setBusy(false);
    }
  }

  if (!isAdmin) return null;

  return (
    <Section title={t("system.mcp.title")} description={t("system.mcp.desc")}>
      <div className="space-y-3">
        <div className="border border-border bg-card/35 px-3 py-2">
          <p className="micro">endpoint</p>
          <p className="mt-1 break-all font-mono text-[11px]">{endpoint}</p>
          <p className="mt-1 font-mono text-[9px] text-muted-foreground">
            Authorization: Bearer rkt_...
          </p>
        </div>
        <div className="flex flex-wrap items-end gap-2">
          <label className="min-w-[12rem] flex-1">
            <span className="micro">{t("system.mcp.name")}</span>
            <Input value={name} onChange={(e) => setName(e.target.value)} className="mt-1 h-8 font-mono text-[12px]" />
          </label>
          <Button size="sm" disabled={busy} onClick={create}>
            <KeyRound className="h-3 w-3" /> {t("system.mcp.create")}
          </Button>
        </div>
        {err && <p className="font-mono text-[10px] text-bad">{err}</p>}
        {created && (
          <div className="space-y-2 border border-ok/35 bg-ok/5 px-3 py-2">
            <p className="font-mono text-[10px] text-ok">{t("system.mcp.once")}</p>
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 break-all font-mono text-[11px]">{created}</code>
              <Button
                size="xs"
                variant="outline"
                onClick={() => copy(created)}
              >
                {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
              </Button>
            </div>
          </div>
        )}
        <div className="border border-border">
          {tokens.length === 0 && (
            <p className="px-3 py-2 font-mono text-[10px] text-muted-foreground">{t("system.mcp.none")}</p>
          )}
          {tokens.map((tok) => (
            <div key={tok.id} className="flex items-center justify-between gap-3 border-b border-border/60 px-3 py-2 last:border-0">
              <div className="min-w-0">
                <p className="truncate font-mono text-[11px]">{tok.name}</p>
                <p className="font-mono text-[9px] text-muted-foreground">
                  {tok.role} · {tok.last_used || t("system.mcp.never")} · #{tok.id}
                </p>
              </div>
              <Button size="xs" variant="outline" onClick={() => revokeToken(tok.id).then(load).catch((e) => setErr(String(e)))}>
                <Trash2 className="h-3 w-3" /> {t("system.mcp.revoke")}
              </Button>
            </div>
          ))}
        </div>
      </div>
    </Section>
  );
}
