import { useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { useI18n } from "../../shared/i18n";
import { tokenizeSql, type TokKind } from "./sql";

const TOK_CLASS: Record<TokKind, string | undefined> = {
  kw: "text-sky-700 dark:text-sky-400",
  str: "text-emerald-700 dark:text-emerald-400",
  comment: "text-zinc-500",
  num: "text-amber-700 dark:text-amber-300",
  fn: "text-violet-700 dark:text-violet-400",
  plain: undefined,
};

// SqlEditor con resaltado propio: textarea transparente sobre <pre>
// coloreado, misma métrica en ambos (mono 12.5px/21px, sin wrap).
// Cmd/Ctrl+Enter ejecuta; Tab indenta. La escritura exige armar
// (doble clic) igual que el resto de acciones destructivas.
export function SqlEditor({
  value,
  onChange,
  onRun,
  running,
  readOnly,
  canRun,
  role,
  elapsedMs,
}: {
  value: string;
  onChange: (v: string) => void;
  onRun: () => void;
  running: boolean;
  readOnly: boolean;
  canRun: boolean;
  role: string;
  elapsedMs: number | null;
}) {
  const { t } = useI18n();
  const [armed, setArmed] = useState(false);
  const taRef = useRef<HTMLTextAreaElement>(null);
  const codeRef = useRef<HTMLElement>(null);
  const gutterRef = useRef<HTMLDivElement>(null);

  const toks = useMemo(() => tokenizeSql(value), [value]);
  const lineCount = useMemo(() => value.split("\n").length, [value]);

  const sync = () => {
    const ta = taRef.current;
    if (!ta) return;
    if (codeRef.current) {
      codeRef.current.style.transform = `translate(${-ta.scrollLeft}px, ${-ta.scrollTop}px)`;
    }
    if (gutterRef.current) {
      gutterRef.current.style.transform = `translateY(${-ta.scrollTop}px)`;
    }
  };

  const submit = () => {
    if (running || !canRun || !value.trim()) return;
    if (!readOnly && !armed) {
      setArmed(true);
      return;
    }
    setArmed(false);
    onRun();
  };

  return (
    <div>
      <div className="relative max-h-[420px] min-h-[220px] overflow-hidden border border-input bg-field/35 font-mono text-[12.5px] leading-[21px] [tab-size:4]">
        <div aria-hidden className="absolute inset-y-0 left-0 z-10 w-11 overflow-hidden border-r border-border/60 bg-card/70 py-2.5">
          <div ref={gutterRef} className="text-right text-muted-foreground/50">
            {Array.from({ length: lineCount }, (_, i) => (
              <div key={i + 1} className="px-2.5">
                {i + 1}
              </div>
            ))}
          </div>
        </div>
        <pre aria-hidden className="m-0 overflow-hidden px-3 py-2.5 pl-[52px] whitespace-pre">
          <code ref={codeRef} className="block w-max">
            {toks.map((tk, i) => (
              <span key={i} className={TOK_CLASS[tk.kind]}>
                {tk.text}
              </span>
            ))}
            {"\u200b"}
          </code>
        </pre>
        <textarea
          ref={taRef}
          value={value}
          onChange={(e) => {
            setArmed(false);
            onChange(e.target.value);
          }}
          onScroll={sync}
          wrap="off"
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          aria-label={t("db.sql")}
          placeholder={t("db.sqlPlaceholder")}
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
              e.preventDefault();
              submit();
            } else if (e.key === "Tab") {
              e.preventDefault();
              const ta = e.currentTarget;
              const s = ta.selectionStart;
              onChange(value.slice(0, s) + "  " + value.slice(ta.selectionEnd));
              requestAnimationFrame(() => ta.setSelectionRange(s + 2, s + 2));
            }
          }}
          className="absolute inset-0 h-full w-full resize-none overflow-auto bg-transparent px-3 py-2.5 pl-[52px] whitespace-pre text-transparent caret-foreground outline-none selection:bg-sky-500/30 placeholder:text-muted-foreground/70"
        />
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          disabled={running || !canRun || !value.trim()}
          onClick={submit}
          className={
            !readOnly && armed
              ? "bg-amber-600 text-white hover:bg-amber-500"
              : "bg-emerald-700 text-white hover:bg-emerald-600 disabled:opacity-50"
          }
          title="Ctrl+Enter"
        >
          {running ? "…" : !readOnly && armed ? t("db.runConfirm") : "Run ⌘⏎"}
        </Button>
        <span
          className={`font-mono text-[10px] uppercase tracking-[0.08em] ${readOnly ? "text-emerald-500/80" : "text-amber-500/90"}`}
        >
          {readOnly ? t("db.readOnly") : t("db.writes")}
        </span>
        <span className="tnum font-mono text-[10px] text-muted-foreground">
          role {role}
          {elapsedMs !== null && ` · ${elapsedMs} ms`}
        </span>
        {!canRun && (
          <span className="font-mono text-[10px] text-muted-foreground">{t("db.needsOperator")}</span>
        )}
      </div>
    </div>
  );
}
