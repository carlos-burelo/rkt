import { useEffect, useMemo, useState } from "react";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { useI18n } from "../../shared/i18n";
import type { PgColumnDetail } from "./api";

// Campos de fila para usar dentro de FormDialog (la regla: crear/editar
// va en diálogo, no inline). Emite el payload y si cambió algo; el padre
// guarda y cierra. NULL explícito por columna con checkbox shadcn.
export function RowFields({
  columns,
  initial,
  mode,
  onChange,
}: {
  columns: PgColumnDetail[];
  initial: Record<string, unknown>;
  mode: "insert" | "edit";
  onChange: (data: Record<string, unknown>, changed: boolean) => void;
}) {
  const { t } = useI18n();
  const strOf = (v: unknown): string => {
    if (v === null || v === undefined) return "";
    if (typeof v === "boolean") return v ? "true" : "false";
    return String(v);
  };
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(columns.map((c) => [c.name, strOf(initial[c.name])])),
  );
  const [nulls, setNulls] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(
      columns.map((c) => [
        c.name,
        initial[c.name] === null || (initial[c.name] === undefined && c.nullable && mode === "insert"),
      ]),
    ),
  );

  const payload = useMemo(() => {
    const out: Record<string, unknown> = {};
    for (const c of columns) {
      if (nulls[c.name]) {
        out[c.name] = null;
        continue;
      }
      out[c.name] = coerce(c.type, values[c.name] ?? "");
    }
    return out;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [columns, values, nulls]);

  const changed = useMemo(() => {
    if (mode === "insert") return true;
    return columns.some((c) => !sameVal(payload[c.name], initial[c.name]));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [columns, payload, mode]);

  useEffect(() => {
    onChange(payload, changed);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [payload, changed]);

  const setNull = (name: string, v: boolean) => setNulls((p) => ({ ...p, [name]: v }));

  const inputFor = (c: PgColumnDetail) => {
    const base = c.type.toLowerCase();
    if (base === "boolean" || base === "bool") {
      const cur = nulls[c.name] ? "__null" : (values[c.name] ?? "");
      return (
        <select
          value={cur}
          onChange={(e) => {
            const v = e.target.value;
            if (v === "__null") setNull(c.name, true);
            else {
              setNull(c.name, false);
              setValues((p) => ({ ...p, [c.name]: v }));
            }
          }}
          className="h-8 w-full border border-input bg-field/35 px-2 font-mono text-[12px]"
          aria-label={c.name}
        >
          {c.nullable && <option value="__null">NULL</option>}
          <option value="true">true</option>
          <option value="false">false</option>
        </select>
      );
    }
    if (base === "jsonb" || base === "json") {
      return (
        <textarea
          value={nulls[c.name] ? "" : (values[c.name] ?? "")}
          disabled={!!nulls[c.name]}
          onChange={(e) => setValues((p) => ({ ...p, [c.name]: e.target.value }))}
          rows={2}
          spellCheck={false}
          placeholder='{"k": 1}'
          aria-label={c.name}
          className="w-full border border-input bg-field/35 px-2 py-1.5 font-mono text-[12px] disabled:opacity-40"
        />
      );
    }
    return (
      <Input
        value={nulls[c.name] ? "" : (values[c.name] ?? "")}
        disabled={!!nulls[c.name]}
        onChange={(e) => setValues((p) => ({ ...p, [c.name]: e.target.value }))}
        aria-label={c.name}
        placeholder={c.default ?? ""}
        className="h-8 font-mono text-[12px]"
      />
    );
  };

  return (
    <div className="grid max-h-[50vh] gap-3 overflow-y-auto pr-1 sm:grid-cols-2">
      {columns.map((c) => (
        <div key={c.name} className="min-w-0">
          <span className="mb-1 flex items-center gap-1.5 font-mono text-[11px]">
            <span className="truncate">{c.name}</span>
            <span className="shrink-0 text-muted-foreground/70">{c.type}</span>
            {c.is_pk && (
              <span className="shrink-0 border border-signal/50 px-1 text-[9px] text-signal">PK</span>
            )}
            {!c.nullable && <span className="shrink-0 text-[9px] text-muted-foreground/60">*</span>}
          </span>
          {inputFor(c)}
          {c.nullable && c.type.toLowerCase() !== "boolean" && c.type.toLowerCase() !== "bool" && (
            <label className="mt-1.5 flex cursor-pointer items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
              <Checkbox checked={!!nulls[c.name]} onCheckedChange={(v) => setNull(c.name, v === true)} />
              {t("db.nullValue")}
            </label>
          )}
        </div>
      ))}
    </div>
  );
}

function coerce(type: string, raw: string): unknown {
  const b = type.toLowerCase();
  if (b === "boolean" || b === "bool") {
    if (raw === "true") return true;
    if (raw === "false") return false;
    return raw;
  }
  return raw;
}

function sameVal(a: unknown, b: unknown): boolean {
  if (a === null || b === null || b === undefined) return a === b || (a === null && b === undefined);
  return String(a) === String(b);
}
