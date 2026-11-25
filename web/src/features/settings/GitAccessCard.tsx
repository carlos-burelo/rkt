import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Plus, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Section } from "../../shared/components/patterns";
import { StatusText } from "../../shared/components/ui";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { CopyButton } from "../../shared/components/feedback";
import { useAuth } from "../auth/store";
import { useI18n } from "../../shared/i18n";
import {
  addGitCred,
  delGitCred,
  listGitCreds,
  revalidateGithub,
  type GitCred,
  type GithubReport,
} from "./api";

// Acceso a repos privados. Preferida: la GitHub App (todos los repos de la
// cuenta y orgs). PAT y deploy key SSH cubren lo que la App no alcanza.
// "Revalidar" detecta orgs nuevas donde falta instalar la App.
export function GitAccessCard() {
  const { t } = useI18n();
  const canWrite = useAuth((a) => a.canWrite);
  const [creds, setCreds] = useState<GitCred[]>([]);
  const [report, setReport] = useState<GithubReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState("");
  const [kind, setKind] = useState<"pat" | "ssh">("ssh");
  const [match, setMatch] = useState("*");
  const [secret, setSecret] = useState("");
  const [deployKey, setDeployKey] = useState("");

  const load = useCallback(() => {
    listGitCreds()
      .then(setCreds)
      .catch(() => setCreds([]));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const revalidate = async () => {
    setBusy(true);
    try {
      setReport(await revalidateGithub());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "error");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Section
      title={t("system.git.title")}
      description={t("system.git.desc")}
      actions={
        <div className="flex items-center gap-1">
          <Button size="xs" variant="outline" disabled={busy} onClick={revalidate}>
            <RefreshCw className="h-3 w-3" /> {busy ? t("system.git.revalidating") : t("system.git.revalidate")}
          </Button>
          {canWrite && (
            <FormDialog
              trigger={
                <Button size="xs" variant="outline">
                  <Plus className="h-3 w-3" /> {t("system.git.add")}
                </Button>
              }
              title={t("system.git.addTitle")}
              description={t("system.git.addDesc")}
              submitLabel={t("common.create")}
              canSubmit={name.trim() !== "" && (kind === "ssh" || secret.trim() !== "")}
              onSubmit={async () => {
                const res = await addGitCred({ name: name.trim(), kind, match: match.trim() || "*", secret: kind === "pat" ? secret.trim() : "" });
                setDeployKey(res.deploy_key ?? "");
                setName("");
                setSecret("");
                load();
              }}
            >
              <Field label={t("system.git.fieldName")}>
                <Input value={name} onChange={(e) => setName(e.target.value)} className="h-8 font-mono text-[12px]" />
              </Field>
              <Field label={t("system.git.fieldKind")}>
                <select
                  value={kind}
                  onChange={(e) => setKind(e.target.value as "pat" | "ssh")}
                  className="h-8 w-full border border-border bg-background px-2 font-mono text-[12px]"
                >
                  <option value="ssh">{t("system.git.kindSsh")}</option>
                  <option value="pat">{t("system.git.kindPat")}</option>
                </select>
              </Field>
              <Field label={t("system.git.fieldMatch")} hint={t("system.git.matchHint")}>
                <Input value={match} onChange={(e) => setMatch(e.target.value)} className="h-8 font-mono text-[12px]" />
              </Field>
              {kind === "pat" && (
                <Field label={t("system.git.fieldToken")} hint={t("system.git.tokenHint")}>
                  <Input
                    type="password"
                    autoComplete="off"
                    value={secret}
                    onChange={(e) => setSecret(e.target.value)}
                    className="h-8 font-mono text-[12px]"
                  />
                </Field>
              )}
            </FormDialog>
          )}
        </div>
      }
    >
      {deployKey && (
        <div className="mb-3 space-y-1 border border-ok/40 bg-ok/5 px-3 py-2 font-mono text-[11px]">
          <p className="text-ok">{t("system.git.deployKeyReady")}</p>
          <p className="break-all text-muted-foreground">{deployKey}</p>
          <CopyButton text={deployKey} />
        </div>
      )}

      <div className="border border-border">
        {creds.map((c) => (
          <div key={c.name} className="flex items-center gap-2 border-b border-border/60 bg-card/35 px-3 py-2 last:border-0">
            <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{c.name}</span>
            <span className="font-mono text-[10px] text-muted-foreground">{c.kind}</span>
            <span className="hidden font-mono text-[10px] text-muted-foreground sm:block">{c.match}</span>
            {canWrite && <ConfirmButton title={t("system.git.remove", { name: c.name })} onConfirm={() => delGitCred(c.name).then(load)} />}
          </div>
        ))}
        {creds.length === 0 && <p className="px-3 py-4 font-mono text-[10px] text-muted-foreground">{t("system.git.none")}</p>}
      </div>

      {report && (
        <div className="mt-3 space-y-2 border border-border px-3 py-2 text-[11px]">
          <StatusText tone={report.healthy ? "ok" : "warn"}>
            {report.healthy ? t("system.git.healthy") : t("system.git.attention")}
          </StatusText>
          <ul className="space-y-0.5 font-mono">
            {report.installations.map((i) => (
              <li key={i.id}>
                {i.account} · {i.selection} · {i.repos} repos
                {i.selection !== "all" && (
                  <a className="ml-2 underline text-warn" href={i.settings_url} target="_blank" rel="noopener noreferrer">
                    {t("system.git.makeAll")}
                  </a>
                )}
              </li>
            ))}
          </ul>
          {report.missing.map((m) => (
            <p key={m.account} className="font-mono text-warn">
              {t("system.git.missing", { account: m.account })}{" "}
              <a className="underline" href={m.install_url} target="_blank" rel="noopener noreferrer">
                {t("system.git.install")}
              </a>
            </p>
          ))}
          {report.notes.map((n) => (
            <p key={n} className="text-muted-foreground">
              {n}
            </p>
          ))}
        </div>
      )}
    </Section>
  );
}
