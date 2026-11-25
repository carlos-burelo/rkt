import { useState } from "react";
import { Input } from "@/components/ui/input";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import { describeTable, type PgTableDetail, type PgTableSchema } from "./api";
import { CreateTableDialog } from "./TableDialogs";

// Explorador de objetos estilo HeidiSQL: buscador + árbol tablas →
// columnas (tipo + PK) con conteo de filas. Seleccionar una tabla abre
// sus datos; el pie crea tablas (diálogo, no inline).
export function TableTree({
  app,
  tables,
  current,
  onSelect,
  onCreated,
}: {
  app: string;
  tables: PgTableDetail[];
  current: string;
  onSelect: (table: string) => void;
  onCreated: () => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [q, setQ] = useState("");
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [schemas, setSchemas] = useState<Record<string, PgTableSchema>>({});
  const [loading, setLoading] = useState<string | null>(null);

  const visible = tables.filter((x) => x.name.toLowerCase().includes(q.trim().toLowerCase()));

  const toggle = async (name: string) => {
    if (expanded.has(name)) {
      setExpanded((p) => {
        const n = new Set(p);
        n.delete(name);
        return n;
      });
      return;
    }
    setExpanded((p) => new Set(p).add(name));
    if (schemas[name] || loading === name) return;
    setLoading(name);
    try {
      const d = await describeTable(app, name);
      setSchemas((p) => ({ ...p, [name]: d.schema }));
    } catch (e) {
      fail(e);
      setExpanded((p) => {
        const n = new Set(p);
        n.delete(name);
        return n;
      });
    } finally {
      setLoading(null);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div className="p-2">
        <Input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t("db.treeSearch")}
          aria-label={t("db.treeSearch")}
          className="h-8 font-mono text-[12px]"
        />
      </div>
      <ul className="min-h-0 flex-1 space-y-px overflow-y-auto px-1 pb-2">
        {visible.length === 0 && (
          <li className="px-2 py-3 font-mono text-[11px] text-muted-foreground">{t("db.noTables")}</li>
        )}
        {visible.map((tb) => {
          const open = expanded.has(tb.name);
          const sc = schemas[tb.name];
          return (
            <li key={tb.name}>
              <div
                className={`flex w-full items-center gap-1.5 px-1.5 py-1.5 text-left hover:bg-accent/40 ${tb.name === current ? "bg-accent/50" : ""}`}
              >
                <button
                  type="button"
                  onClick={() => void toggle(tb.name)}
                  aria-label={open ? t("db.treeCollapse", { table: tb.name }) : t("db.treeExpand", { table: tb.name })}
                  aria-expanded={open}
                  className="shrink-0 px-1 font-mono text-[11px] text-muted-foreground hover:text-foreground"
                >
                  {open ? "▾" : "▸"}
                </button>
                <button
                  type="button"
                  onClick={() => onSelect(tb.name)}
                  className="flex min-w-0 flex-1 items-center gap-2 text-left"
                  title={tb.name}
                >
                  <span className="truncate font-mono text-[12px]">{tb.name}</span>
                </button>
                {tb.rows >= 0 && (
                  <span className="tnum shrink-0 font-mono text-[10px] text-muted-foreground">{tb.rows}</span>
                )}
              </div>
              {open && (
                <ul className="ml-5 space-y-px border-l border-border/70 py-1 pl-2">
                  {loading === tb.name && !sc && (
                    <li className="px-1.5 py-1 font-mono text-[10px] text-muted-foreground">…</li>
                  )}
                  {sc?.columns.map((c) => (
                    <li
                      key={c.name}
                      className="flex items-center gap-1.5 px-1.5 py-1"
                      title={`${c.name}: ${c.type}${c.is_pk ? " (PK)" : ""}`}
                    >
                      <span className="truncate font-mono text-[11px] text-muted-foreground">{c.name}</span>
                      <span className="shrink-0 font-mono text-[10px] text-muted-foreground/60">{c.type}</span>
                      {c.is_pk && (
                        <span className="shrink-0 border border-signal/50 px-1 font-mono text-[9px] text-signal">
                          PK
                        </span>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </li>
          );
        })}
      </ul>
      <div className="border-t border-border p-2">
        <CreateTableDialog app={app} onCreated={onCreated} />
      </div>
    </div>
  );
}
