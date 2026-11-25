import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { FormDialog, Field } from "../../shared/components/FormDialog";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import { addColumn, createTable, provisionPg, createIndex, createPolicy, renameTable, duplicateTable, renameColumn, alterColumn, createView, createTrigger, COLUMN_TYPES, type ColumnDef, type PgColumnDetail } from "./api";

// Diálogos DDL del studio: crear tabla y añadir columna. La regla es que
// la carga de datos va en diálogo (FormDialog), con aire: una columna por
// fila, tipos con select ancho y validación antes de enviar.

export function CreateTableDialog({ app, onCreated }: { app: string; onCreated: () => void }) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [name, setName] = useState("");
  const [cols, setCols] = useState<ColumnDef[]>([
    { name: "id", type: "uuid", pk: true, default: "gen_random_uuid()" },
  ]);

  const valid =
    name.trim() !== "" && cols.length > 0 && cols.every((c) => c.name.trim() !== "");

  const submit = async () => {
    try {
      await createTable(app, name.trim().toLowerCase(), cols.filter((c) => c.name.trim()));
      toast.success(name.trim());
      setName("");
      setCols([{ name: "", type: "text" }]);
      onCreated();
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  return (
    <FormDialog
      trigger={
        <Button size="sm" className="w-full">
          + {t("db.createTable")}
        </Button>
      }
      title={t("db.createTableTitle")}
      description={t("db.createTableDesc")}
      submitLabel={t("db.createTable")}
      canSubmit={valid}
      onSubmit={submit}
      className="sm:max-w-2xl"
    >
      <Field label={t("db.tableName")}>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="pedidos"
          aria-label={t("db.tableName")}
          className="h-8 font-mono text-[12px]"
          autoFocus
        />
      </Field>
      <div className="space-y-2">
        <p className="micro text-muted-foreground">{t("db.columnsLabel")}</p>
        {cols.map((c, i) => (
          <div
            key={i}
            className="grid grid-cols-[1fr_9rem_auto] items-end gap-2 border border-border/70 bg-card/40 p-2"
          >
            <Field label={t("db.colName")}>
              <Input
                value={c.name}
                onChange={(e) =>
                  setCols((prev) => prev.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))
                }
                placeholder="cliente_id"
                aria-label={t("db.colName")}
                className="h-8 font-mono text-[12px]"
              />
            </Field>
            <Field label={t("db.colType")}>
              <select
                value={c.type}
                onChange={(e) =>
                  setCols((prev) => prev.map((x, j) => (j === i ? { ...x, type: e.target.value } : x)))
                }
                className="h-8 w-full border border-input bg-field/35 px-1.5 font-mono text-[11px]"
                aria-label={t("db.colType")}
              >
                {COLUMN_TYPES.map((ct) => (
                  <option key={ct} value={ct}>
                    {ct}
                  </option>
                ))}
              </select>
            </Field>
            <div className="flex items-center gap-3 pb-1.5">
              <label className="flex cursor-pointer items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
                <Checkbox
                  checked={!!c.pk}
                  onCheckedChange={(v) =>
                    setCols((prev) => prev.map((x, j) => (j === i ? { ...x, pk: v === true } : x)))
                  }
                />
                PK
              </label>
              <label className="flex cursor-pointer items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
                <Checkbox
                  checked={!!c.not_null}
                  onCheckedChange={(v) =>
                    setCols((prev) => prev.map((x, j) => (j === i ? { ...x, not_null: v === true } : x)))
                  }
                />
                NOT NULL
              </label>
              <Button
                size="sm"
                variant="outline"
                disabled={cols.length <= 1}
                onClick={() => setCols((prev) => prev.filter((_, j) => j !== i))}
                aria-label={t("common.delete")}
                type="button"
              >
                ×
              </Button>
            </div>
            <div className="col-span-3">
              <Field label={t("db.colDefault")} hint={t("db.colDefaultHint")}>
                <Input
                  value={c.default ?? ""}
                  onChange={(e) =>
                    setCols((prev) =>
                      prev.map((x, j) => (j === i ? { ...x, default: e.target.value || undefined } : x)),
                    )
                  }
                  placeholder="now()"
                  aria-label={t("db.colDefault")}
                  className="h-8 font-mono text-[12px]"
                />
              </Field>
            </div>
          </div>
        ))}
        <Button
          size="sm"
          variant="outline"
          type="button"
          disabled={cols.length >= 20}
          onClick={() => setCols((prev) => [...prev, { name: "", type: "text" }])}
        >
          + {t("db.addColumnRow")}
        </Button>
      </div>
    </FormDialog>
  );
}

export function AddColumnDialog({ app,
  table,
  onCreated,
}: {
  app: string;
  table: string;
  onCreated: () => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [name, setName] = useState("");
  const [type, setType] = useState<string>("text");
  const [notNull, setNotNull] = useState(false);
  const [def, setDef] = useState("");

  const submit = async () => {
    try {
      await addColumn(app, table, {
        name: name.trim().toLowerCase(),
        type,
        not_null: notNull || undefined,
        default: def.trim() || undefined,
      });
      toast.success(name.trim());
      setName("");
      setDef("");
      setNotNull(false);
      onCreated();
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  return (
    <FormDialog
      trigger={
        <Button size="sm" variant="outline">
          + {t("db.addColumn")}
        </Button>
      }
      title={t("db.addColumnTitle", { table })}
      submitLabel={t("db.addColumn")}
      canSubmit={name.trim() !== ""}
      onSubmit={submit}
    >
      <Field label={t("db.colName")}>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          aria-label={t("db.colName")}
          className="h-8 font-mono text-[12px]"
          autoFocus
        />
      </Field>
      <Field label={t("db.colType")}>
        <select
          value={type}
          onChange={(e) => setType(e.target.value)}
          className="h-8 w-full border border-input bg-field/35 px-1.5 font-mono text-[11px]"
          aria-label={t("db.colType")}
        >
          {COLUMN_TYPES.map((ct) => (
            <option key={ct} value={ct}>
              {ct}
            </option>
          ))}
        </select>
      </Field>
      <label className="flex cursor-pointer items-center gap-2 font-mono text-[11px] text-muted-foreground">
        <Checkbox checked={notNull} onCheckedChange={(v) => setNotNull(v === true)} />
        NOT NULL
      </label>
      <Field label={t("db.colDefault")} hint={t("db.colDefaultHint")}>
        <Input
          value={def}
          onChange={(e) => setDef(e.target.value)}
          placeholder="now()"
          aria-label={t("db.colDefault")}
          className="h-8 font-mono text-[12px]"
        />
      </Field>
    </FormDialog>
  );
}

export function ProvisionDialog({
  onCreated,
}: {
  onCreated: (app: string) => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [name, setName] = useState("");
  const [mode, setMode] = useState("dedicated");

  const submit = async () => {
    const app = name.trim().toLowerCase();
    try {
      await provisionPg(app, mode);
      toast.success(app);
      setName("");
      onCreated(app);
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  return (
    <FormDialog
      trigger={
        <Button size="sm" variant="outline" className="w-full">
          + {t("db.provisionTitle")}
        </Button>
      }
      title={t("db.provisionTitle")}
      description={t("db.provisionDesc")}
      submitLabel={t("db.provision")}
      canSubmit={name.trim() !== ""}
      onSubmit={submit}
    >
      <Field label={t("db.appPlaceholder")}>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="app"
          aria-label={t("db.appPlaceholder")}
          className="h-8 font-mono text-[12px]"
          autoFocus
        />
      </Field>
      <Field label={t("db.mode")}>
        <select
          value={mode}
          onChange={(e) => setMode(e.target.value)}
          aria-label={t("db.mode")}
          className="h-8 w-full border border-input bg-field/35 px-1.5 font-mono text-[11px]"
        >
          <option value="dedicated">dedicated</option>
          <option value="shared">shared</option>
        </select>
      </Field>
    </FormDialog>
  );
}

// Diálogos DDL avanzada del inspector: índices, policies, renombrar,
// duplicar y alterar columnas. Mismo estilo FormDialog que arriba.

export function CreateIndexDialog({
  app,
  table,
  columns,
  onCreated,
}: {
  app: string;
  table: string;
  columns?: string[];
  onCreated: () => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [name, setName] = useState("");
  const [colsText, setColsText] = useState("");
  const [sel, setSel] = useState<string[]>([]);
  const [unique, setUnique] = useState(false);
  const [method, setMethod] = useState("");

  const picked = sel.length ? sel : colsText.split(",").map((c) => c.trim().toLowerCase()).filter(Boolean);
  const valid = picked.length > 0;

  const toggle = (c: string) => {
    setSel((prev) => (prev.includes(c) ? prev.filter((x) => x !== c) : [...prev, c]));
  };

  const submit = async () => {
    try {
      await createIndex(app, table, {
        name: name.trim() || undefined,
        columns: picked,
        unique: unique || undefined,
        method: method || undefined,
      });
      toast.success(name.trim() || picked.join("_"));
      setName("");
      setColsText("");
      setSel([]);
      setUnique(false);
      setMethod("");
      onCreated();
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  return (
    <FormDialog
      trigger={
        <Button size="sm" variant="outline">
          + {t("db.createIndex")}
        </Button>
      }
      title={t("db.createIndexTitle", { table })}
      submitLabel={t("db.createIndex")}
      canSubmit={valid}
      onSubmit={submit}
    >
      <Field label={t("db.indexName")} hint={t("db.optional")}>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={`${table}_${picked[0] ?? "col"}_idx`}
          aria-label={t("db.indexName")}
          className="h-8 font-mono text-[12px]"
          autoFocus
        />
      </Field>
      {columns && columns.length > 0 ? (
        <div className="space-y-1.5">
          <p className="micro text-muted-foreground">{t("db.columnsLabel")}</p>
          <div className="flex flex-wrap gap-1.5">
            {columns.map((c) => (
              <label
                key={c}
                className={`flex cursor-pointer items-center gap-1.5 border px-1.5 py-1 font-mono text-[11px] ${sel.includes(c) ? "border-signal/60 text-foreground" : "border-border text-muted-foreground"}`}
              >
                <Checkbox checked={sel.includes(c)} onCheckedChange={() => toggle(c)} />
                {c}
              </label>
            ))}
          </div>
        </div>
      ) : (
        <Field label={t("db.columnsLabel")} hint="col1, col2">
          <Input
            value={colsText}
            onChange={(e) => setColsText(e.target.value)}
            placeholder="cliente_id, creada"
            aria-label={t("db.columnsLabel")}
            className="h-8 font-mono text-[12px]"
            autoFocus
          />
        </Field>
      )}
      <div className="flex items-center gap-4">
        <label className="flex cursor-pointer items-center gap-2 font-mono text-[11px] text-muted-foreground">
          <Checkbox checked={unique} onCheckedChange={(v) => setUnique(v === true)} />
          UNIQUE
        </label>
        <Field label={t("db.indexMethod")} hint={t("db.optional")}>
          <select
            value={method}
            onChange={(e) => setMethod(e.target.value)}
            className="h-8 w-32 border border-input bg-field/35 px-1.5 font-mono text-[11px]"
            aria-label={t("db.indexMethod")}
          >
            <option value="">btree</option>
            {["btree", "hash", "gin", "gist", "brin"].map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
        </Field>
      </div>
    </FormDialog>
  );
}

export function CreatePolicyDialog({
  app,
  table,
  onCreated,
}: {
  app: string;
  table: string;
  columns?: string[];
  onCreated: () => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [name, setName] = useState("");
  const [cmd, setCmd] = useState("ALL");
  const [roles, setRoles] = useState("");
  const [using, setUsing] = useState("");
  const [check, setCheck] = useState("");

  const submit = async () => {
    try {
      await createPolicy(app, table, {
        name: name.trim(),
        cmd,
        roles: roles.split(",").map((r) => r.trim()).filter(Boolean),
        using: using.trim() || undefined,
        check: check.trim() || undefined,
      });
      toast.success(name.trim());
      setName("");
      setRoles("");
      setUsing("");
      setCheck("");
      setCmd("ALL");
      onCreated();
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  return (
    <FormDialog
      trigger={
        <Button size="sm" variant="outline">
          + {t("db.createPolicy")}
        </Button>
      }
      title={t("db.createPolicyTitle", { table })}
      submitLabel={t("db.createPolicy")}
      canSubmit={name.trim() !== ""}
      onSubmit={submit}
    >
      <Field label={t("db.policyName")}>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="lectura_propia"
          aria-label={t("db.policyName")}
          className="h-8 font-mono text-[12px]"
          autoFocus
        />
      </Field>
      <div className="grid grid-cols-2 gap-2">
        <Field label={t("db.policyCmd")}>
          <select
            value={cmd}
            onChange={(e) => setCmd(e.target.value)}
            className="h-8 w-full border border-input bg-field/35 px-1.5 font-mono text-[11px]"
            aria-label={t("db.policyCmd")}
          >
            {["ALL", "SELECT", "INSERT", "UPDATE", "DELETE"].map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t("db.policyRoles")} hint={t("db.policyRolesHint")}>
          <Input
            value={roles}
            onChange={(e) => setRoles(e.target.value)}
            placeholder="app_rol, anon"
            aria-label={t("db.policyRoles")}
            className="h-8 font-mono text-[12px]"
          />
        </Field>
      </div>
      <Field label="USING" hint={t("db.optional")}>
        <Input
          value={using}
          onChange={(e) => setUsing(e.target.value)}
          placeholder="auth.uid() = dueno_id"
          aria-label="USING"
          className="h-8 font-mono text-[12px]"
        />
      </Field>
      <Field label="WITH CHECK" hint={t("db.optional")}>
        <Input
          value={check}
          onChange={(e) => setCheck(e.target.value)}
          placeholder="auth.uid() = dueno_id"
          aria-label="WITH CHECK"
          className="h-8 font-mono text-[12px]"
        />
      </Field>
    </FormDialog>
  );
}

export function RenameTableDialog({
  app,
  table,
  onCreated,
}: {
  app: string;
  table: string;
  columns?: string[];
  onCreated: () => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [name, setName] = useState("");

  const submit = async () => {
    try {
      await renameTable(app, table, name.trim().toLowerCase());
      toast.success(name.trim());
      setName("");
      onCreated();
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  return (
    <FormDialog
      trigger={
        <Button size="sm" variant="outline">
          {t("db.renameTable")}
        </Button>
      }
      title={t("db.renameTableTitle", { table })}
      submitLabel={t("db.renameTable")}
      canSubmit={name.trim() !== "" && name.trim().toLowerCase() !== table}
      onSubmit={submit}
    >
      <Field label={t("db.newName")}>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={`${table}_v2`}
          aria-label={t("db.newName")}
          className="h-8 font-mono text-[12px]"
          autoFocus
        />
      </Field>
    </FormDialog>
  );
}

export function DuplicateTableDialog({
  app,
  table,
  onCreated,
}: {
  app: string;
  table: string;
  columns?: string[];
  onCreated: () => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [name, setName] = useState(`${table}_copia`);
  const [withData, setWithData] = useState(true);

  const submit = async () => {
    try {
      await duplicateTable(app, table, name.trim().toLowerCase(), withData);
      toast.success(name.trim());
      setName(`${table}_copia`);
      setWithData(true);
      onCreated();
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  return (
    <FormDialog
      trigger={
        <Button size="sm" variant="outline">
          {t("db.duplicate")}
        </Button>
      }
      title={t("db.duplicateTitle", { table })}
      submitLabel={t("db.duplicate")}
      canSubmit={name.trim() !== "" && name.trim().toLowerCase() !== table}
      onSubmit={submit}
    >
      <Field label={t("db.newName")}>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          aria-label={t("db.newName")}
          className="h-8 font-mono text-[12px]"
          autoFocus
        />
      </Field>
      <label className="flex cursor-pointer items-center gap-2 font-mono text-[11px] text-muted-foreground">
        <Checkbox checked={withData} onCheckedChange={(v) => setWithData(v === true)} />
        {t("db.withData")}
      </label>
    </FormDialog>
  );
}

export function AlterColumnDialog({
  app,
  table,
  column,
  columns,
  onCreated,
}: {
  app: string;
  table: string;
  column: string;
  columns?: PgColumnDetail[];
  onCreated: () => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const cur = columns?.find((c) => c.name === column);
  const [type, setType] = useState("");
  const [def, setDef] = useState("");
  const [dropDef, setDropDef] = useState(false);
  const [notNull, setNotNull] = useState("");

  const valid =
    type.trim() !== "" || (!dropDef && def.trim() !== "") || dropDef || notNull !== "";

  const submit = async () => {
    try {
      if (type.trim() !== "") {
        await alterColumn(app, table, column, { type: type.trim() });
      }
      if (dropDef) {
        await alterColumn(app, table, column, { drop_default: true });
      } else if (def.trim() !== "") {
        await alterColumn(app, table, column, { default: def.trim() });
      }
      if (notNull !== "") {
        await alterColumn(app, table, column, { not_null: notNull === "set" });
      }
      toast.success(column);
      setType("");
      setDef("");
      setDropDef(false);
      setNotNull("");
      onCreated();
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  return (
    <FormDialog
      trigger={
        <Button size="sm" variant="ghost" className="font-mono text-[11px] text-muted-foreground">
          {t("db.alterColumn")}
        </Button>
      }
      title={t("db.alterColumnTitle", { table, column })}
      description={
        cur ? `${cur.type} · ${cur.nullable ? "NULL" : "NOT NULL"} · ${cur.default ?? "—"}` : undefined
      }
      submitLabel={t("db.alterColumn")}
      canSubmit={valid}
      onSubmit={submit}
    >
      <Field label={t("db.colType")} hint={t("db.optional")}>
        <select
          value={type}
          onChange={(e) => setType(e.target.value)}
          className="h-8 w-full border border-input bg-field/35 px-1.5 font-mono text-[11px]"
          aria-label={t("db.colType")}
        >
          <option value="">—</option>
          {COLUMN_TYPES.map((ct) => (
            <option key={ct} value={ct}>
              {ct}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t("db.colDefault")} hint={t("db.optional")}>
        <Input
          value={def}
          onChange={(e) => setDef(e.target.value)}
          placeholder="now()"
          disabled={dropDef}
          aria-label={t("db.colDefault")}
          className="h-8 font-mono text-[12px]"
        />
      </Field>
      <label className="flex cursor-pointer items-center gap-2 font-mono text-[11px] text-muted-foreground">
        <Checkbox checked={dropDef} onCheckedChange={(v) => setDropDef(v === true)} />
        {t("db.dropDefault")}
      </label>
      <Field label="NOT NULL" hint={t("db.optional")}>
        <select
          value={notNull}
          onChange={(e) => setNotNull(e.target.value)}
          className="h-8 w-full border border-input bg-field/35 px-1.5 font-mono text-[11px]"
          aria-label="NOT NULL"
        >
          <option value="">—</option>
          <option value="set">SET NOT NULL</option>
          <option value="drop">DROP NOT NULL</option>
        </select>
      </Field>
    </FormDialog>
  );
}

export function RenameColumnInline({
  app,
  table,
  column,
  onCreated,
}: {
  app: string;
  table: string;
  column: string;
  columns?: string[];
  onCreated: () => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [name, setName] = useState(column);

  const submit = async () => {
    try {
      await renameColumn(app, table, column, name.trim().toLowerCase());
      toast.success(name.trim());
      onCreated();
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  return (
    <FormDialog
      trigger={
        <Button size="sm" variant="ghost" className="font-mono text-[11px] text-muted-foreground">
          {t("db.renameColumn")}
        </Button>
      }
      title={t("db.renameColumnTitle", { table, column })}
      submitLabel={t("db.renameColumn")}
      canSubmit={name.trim() !== "" && name.trim().toLowerCase() !== column}
      onSubmit={submit}
    >
      <Field label={t("db.newName")}>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          aria-label={t("db.newName")}
          className="h-8 font-mono text-[12px]"
          autoFocus
        />
      </Field>
    </FormDialog>
  );
}

// Diálogos de objetos: vistas y triggers. Mismo estilo FormDialog;
// la tabla del trigger viene precargada desde el filtro de la vista.
export function CreateViewDialog({ app, onCreated }: { app: string; onCreated: () => void }) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [name, setName] = useState("");
  const [sql, setSql] = useState("");
  const [materialized, setMaterialized] = useState(false);
  const [withData, setWithData] = useState(true);
  const [orReplace, setOrReplace] = useState(false);

  const valid = name.trim() !== "" && sql.trim() !== "";

  const submit = async () => {
    try {
      await createView(app, name.trim().toLowerCase(), sql.trim(), {
        materialized: materialized || undefined,
        or_replace: orReplace || undefined,
        with_data: materialized && withData ? true : undefined,
      });
      toast.success(name.trim());
      setName("");
      setSql("");
      setMaterialized(false);
      setWithData(true);
      setOrReplace(false);
      onCreated();
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  return (
    <FormDialog
      trigger={
        <Button size="sm" variant="outline">
          + {t("db.createView")}
        </Button>
      }
      title={t("db.createViewTitle")}
      submitLabel={t("db.createView")}
      canSubmit={valid}
      onSubmit={submit}
    >
      <Field label={t("db.viewName")}>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="vista_pedidos"
          aria-label={t("db.viewName")}
          className="h-8 font-mono text-[12px]"
          autoFocus
        />
      </Field>
      <Field label={t("db.viewSql")} hint={t("db.viewSqlHint")}>
        <textarea
          value={sql}
          onChange={(e) => setSql(e.target.value)}
          placeholder="SELECT id, total FROM pedidos WHERE total > 0"
          aria-label={t("db.viewSql")}
          rows={5}
          className="w-full border border-input bg-field/35 px-2 py-1.5 font-mono text-[12px]"
        />
      </Field>
      <div className="flex flex-wrap items-center gap-4">
        <label className="flex cursor-pointer items-center gap-2 font-mono text-[11px] text-muted-foreground">
          <Checkbox checked={materialized} onCheckedChange={(v) => setMaterialized(v === true)} />
          {t("db.materialized")}
        </label>
        <label className="flex cursor-pointer items-center gap-2 font-mono text-[11px] text-muted-foreground">
          <Checkbox checked={orReplace} onCheckedChange={(v) => setOrReplace(v === true)} />
          {t("db.orReplace")}
        </label>
        {materialized && (
          <label className="flex cursor-pointer items-center gap-2 font-mono text-[11px] text-muted-foreground">
            <Checkbox checked={withData} onCheckedChange={(v) => setWithData(v === true)} />
            {t("db.withData")}
          </label>
        )}
      </div>
    </FormDialog>
  );
}

const TRIGGER_TIMINGS = ["BEFORE", "AFTER", "INSTEAD OF"] as const;
const TRIGGER_EVENTS = ["INSERT", "UPDATE", "DELETE", "TRUNCATE"] as const;

export function CreateTriggerDialog({
  app,
  table,
  tables,
  onCreated,
}: {
  app: string;
  table: string;
  tables?: string[];
  onCreated: () => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [tbl, setTbl] = useState(table);
  const [name, setName] = useState("");
  const [timing, setTiming] = useState<string>("AFTER");
  const [events, setEvents] = useState<string[]>(["INSERT"]);
  const [orientation, setOrientation] = useState("ROW");
  const [fn, setFn] = useState("");
  const [condition, setCondition] = useState("");

  const valid = tbl.trim() !== "" && name.trim() !== "" && events.length > 0 && fn.trim() !== "";

  const toggleEvent = (ev: string) => {
    setEvents((prev) => (prev.includes(ev) ? prev.filter((x) => x !== ev) : [...prev, ev]));
  };

  const submit = async () => {
    try {
      await createTrigger(app, tbl.trim().toLowerCase(), {
        name: name.trim().toLowerCase(),
        timing,
        events,
        orientation,
        function: fn.trim(),
        condition: condition.trim() || undefined,
      });
      toast.success(name.trim());
      setName("");
      setFn("");
      setCondition("");
      setEvents(["INSERT"]);
      setTiming("AFTER");
      setOrientation("ROW");
      onCreated();
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  return (
    <FormDialog
      trigger={
        <Button size="sm" variant="outline">
          + {t("db.createTrigger")}
        </Button>
      }
      title={t("db.createTriggerTitle", { table: tbl || table })}
      submitLabel={t("db.createTrigger")}
      canSubmit={valid}
      onSubmit={submit}
    >
      <Field label={t("db.triggerTable")}>
        {tables && tables.length > 0 ? (
          <select
            value={tbl}
            onChange={(e) => setTbl(e.target.value)}
            className="h-8 w-full border border-input bg-field/35 px-1.5 font-mono text-[11px]"
            aria-label={t("db.triggerTable")}
          >
            {tables.map((tb) => (
              <option key={tb} value={tb}>
                {tb}
              </option>
            ))}
          </select>
        ) : (
          <Input
            value={tbl}
            onChange={(e) => setTbl(e.target.value)}
            aria-label={t("db.triggerTable")}
            className="h-8 font-mono text-[12px]"
            autoFocus
          />
        )}
      </Field>
      <Field label={t("db.triggerName")}>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="trg_auditar"
          aria-label={t("db.triggerName")}
          className="h-8 font-mono text-[12px]"
        />
      </Field>
      <div className="grid grid-cols-2 gap-2">
        <Field label={t("db.triggerTiming")}>
          <select
            value={timing}
            onChange={(e) => setTiming(e.target.value)}
            className="h-8 w-full border border-input bg-field/35 px-1.5 font-mono text-[11px]"
            aria-label={t("db.triggerTiming")}
          >
            {TRIGGER_TIMINGS.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t("db.triggerOrientation")}>
          <select
            value={orientation}
            onChange={(e) => setOrientation(e.target.value)}
            className="h-8 w-full border border-input bg-field/35 px-1.5 font-mono text-[11px]"
            aria-label={t("db.triggerOrientation")}
          >
            {["ROW", "STATEMENT"].map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
        </Field>
      </div>
      <div className="space-y-1.5">
        <p className="micro text-muted-foreground">{t("db.triggerEvents")}</p>
        <div className="flex flex-wrap gap-1.5">
          {TRIGGER_EVENTS.map((ev) => (
            <label
              key={ev}
              className={`flex cursor-pointer items-center gap-1.5 border px-1.5 py-1 font-mono text-[11px] ${events.includes(ev) ? "border-signal/60 text-foreground" : "border-border text-muted-foreground"}`}
            >
              <Checkbox checked={events.includes(ev)} onCheckedChange={() => toggleEvent(ev)} />
              {ev}
            </label>
          ))}
        </div>
      </div>
      <Field label={t("db.triggerFunction")} hint={t("db.triggerFunctionHint")}>
        <Input
          value={fn}
          onChange={(e) => setFn(e.target.value)}
          placeholder="auditar_pedidos()"
          aria-label={t("db.triggerFunction")}
          className="h-8 font-mono text-[12px]"
        />
      </Field>
      <Field label={t("db.triggerCondition")} hint={t("db.optional")}>
        <Input
          value={condition}
          onChange={(e) => setCondition(e.target.value)}
          placeholder="OLD.total IS DISTINCT FROM NEW.total"
          aria-label={t("db.triggerCondition")}
          className="h-8 font-mono text-[12px]"
        />
      </Field>
    </FormDialog>
  );
}
