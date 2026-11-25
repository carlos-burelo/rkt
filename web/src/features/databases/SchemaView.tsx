import { useEffect, useState } from "react";
import { Section } from "../../shared/components/patterns";
import { ConfirmButton } from "../../shared/components/ConfirmButton";
import { useUI } from "../../app/ui";
import { useI18n } from "../../shared/i18n";
import {
  dropColumn,
  dropTable,
  dropIndex,
  dropPolicy,
  truncateTable,
  listIndexes,
  listConstraints,
  setRLS,
  type PgTableSchema,
  type PgPolicy,
  type IndexInfo,
  type ConstraintInfo,
} from "./api";
import {
  AddColumnDialog,
  AlterColumnDialog,
  CreateIndexDialog,
  CreatePolicyDialog,
  DuplicateTableDialog,
  RenameColumnInline,
  RenameTableDialog,
} from "./TableDialogs";

// Inspector + DDL estilo Supabase: columnas con PK/nulo/default, índices,
// constraints, RLS con policies en claro. Crear tabla y añadir columna van
// en diálogo (FormDialog); borrar usa ConfirmButton de dos pasos.
export function SchemaView({
  app,
  schema,
  policies,
  canWrite,
  onChanged,
}: {
  app: string;
  schema: PgTableSchema | null;
  policies: PgPolicy[];
  canWrite: boolean;
  onChanged: () => void;
}) {
  const { t } = useI18n();
  const fail = useUI((s) => s.fail);
  const [indexes, setIndexes] = useState<IndexInfo[]>([]);
  const [constraints, setConstraints] = useState<ConstraintInfo[]>([]);

  const table = schema?.table ?? "";

  useEffect(() => {
    if (!table) {
      setIndexes([]);
      setConstraints([]);
      return;
    }
    listIndexes(app, table).then(setIndexes).catch((e) => fail(e));
    listConstraints(app, table).then(setConstraints).catch((e) => fail(e));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [app, table]);

  const refreshMeta = async () => {
    if (!table) return;
    try {
      const [ix, cs] = await Promise.all([listIndexes(app, table), listConstraints(app, table)]);
      setIndexes(ix);
      setConstraints(cs);
    } catch (e) {
      fail(e);
    }
  };

  const afterChange = () => {
    void refreshMeta();
    onChanged();
  };

  const doDropColumn = async (col: string) => {
    if (!schema) return;
    try {
      await dropColumn(app, schema.table, col);
      afterChange();
    } catch (e) {
      fail(e);
    }
  };

  const doDropTable = async () => {
    if (!schema) return;
    try {
      await dropTable(app, schema.table);
      onChanged();
    } catch (e) {
      fail(e);
    }
  };

  const doTruncate = async () => {
    if (!schema) return;
    try {
      await truncateTable(app, schema.table);
      afterChange();
    } catch (e) {
      fail(e);
    }
  };

  const doDropIndex = async (name: string) => {
    try {
      await dropIndex(app, name);
      afterChange();
    } catch (e) {
      fail(e);
    }
  };

  const doDropPolicy = async (name: string) => {
    if (!schema) return;
    try {
      await dropPolicy(app, schema.table, name);
      onChanged();
    } catch (e) {
      fail(e);
    }
  };

  const doRLS = async (enable: boolean) => {
    if (!schema) return;
    try {
      await setRLS(app, schema.table, enable);
      onChanged();
    } catch (e) {
      fail(e);
    }
  };

  if (!schema) return null;

  const colNames = schema.columns.map((c) => c.name);

  return (
    <div className="space-y-4">
      <Section
        title={`${t("db.schema")}: ${schema.table}`}
        description={`${schema.rows} ${t("db.rowsLabel")} · PK: ${schema.pk.length ? schema.pk.join(", ") : "—"}`}
        actions={
          canWrite ? (
            <div className="flex flex-wrap gap-2">
              <AddColumnDialog app={app} table={schema.table} onCreated={afterChange} />
              <RenameTableDialog app={app} table={schema.table} onCreated={onChanged} />
              <DuplicateTableDialog app={app} table={schema.table} onCreated={onChanged} />
              <ConfirmButton
                size="sm"
                label={t("db.truncate")}
                confirmLabel={t("db.truncateConfirm")}
                onConfirm={doTruncate}
              />
              <ConfirmButton
                size="sm"
                label={t("db.dropTable")}
                confirmLabel={t("db.dropTableConfirm")}
                onConfirm={doDropTable}
              />
            </div>
          ) : undefined
        }
      >
        <div className="overflow-x-auto border border-border">
          <table className="w-full text-left font-mono text-[12px]">
            <thead>
              <tr className="border-b border-border bg-secondary/40">
                <th className="px-3 py-2 font-medium">{t("db.colName")}</th>
                <th className="px-3 py-2 font-medium">{t("db.colType")}</th>
                <th className="px-3 py-2 font-medium">{t("db.colNull")}</th>
                <th className="px-3 py-2 font-medium">{t("db.colDefault")}</th>
                <th className="px-3 py-2 font-medium">{t("db.colKey")}</th>
                {canWrite && <th className="w-40 px-3 py-2" />}
              </tr>
            </thead>
            <tbody>
              {schema.columns.map((c) => (
                <tr key={c.name} className="border-b border-border/50 last:border-0 hover:bg-accent/20">
                  <td className="px-3 py-2">{c.name}</td>
                  <td className="px-3 py-2 text-muted-foreground">{c.type}</td>
                  <td className="px-3 py-2 text-muted-foreground">{c.nullable ? "NULL" : "NOT NULL"}</td>
                  <td
                    className="max-w-[260px] truncate px-3 py-2 text-muted-foreground"
                    title={c.default ?? ""}
                  >
                    {c.default ?? <span className="opacity-50">—</span>}
                  </td>
                  <td className="px-3 py-2">
                    {c.is_pk && (
                      <span className="border border-signal/50 px-1 text-[9px] text-signal">PK</span>
                    )}
                  </td>
                  {canWrite && (
                    <td className="px-3 py-2 text-right whitespace-nowrap">
                      <AlterColumnDialog
                        app={app}
                        table={schema.table}
                        column={c.name}
                        columns={schema.columns}
                        onCreated={afterChange}
                      />
                      <RenameColumnInline
                        app={app}
                        table={schema.table}
                        column={c.name}
                        onCreated={afterChange}
                      />
                      <ConfirmButton
                        size="sm"
                        label="×"
                        confirmLabel={t("db.dropColumnConfirm")}
                        onConfirm={() => doDropColumn(c.name)}
                      />
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <span
            className={`font-mono text-[10px] uppercase tracking-[0.08em] ${schema.rls ? "text-ok" : "text-muted-foreground"}`}
          >
            RLS {schema.rls ? "on" : "off"}
            {schema.rls_force ? " · force" : ""}
          </span>
          {canWrite && (
            <ConfirmButton
              size="sm"
              label={schema.rls ? t("db.rlsOff") : t("db.rlsOn")}
              confirmLabel={t("db.runConfirm")}
              onConfirm={() => doRLS(!schema.rls)}
            />
          )}
          {schema.rls && (
            <span className="font-mono text-[10px] text-muted-foreground">
              {t("db.policies", { n: policies.length })}
            </span>
          )}
          {canWrite && (
            <CreatePolicyDialog app={app} table={schema.table} onCreated={onChanged} />
          )}
        </div>
        {policies.length > 0 && (
          <div className="mt-2">
            <p className="micro mb-1 text-muted-foreground">{t("db.policiesTitle")}</p>
          <ul className="space-y-1">
            {policies.map((p) => (
              <li
                key={p.name}
                className="flex items-center gap-2 font-mono text-[11px] text-muted-foreground"
              >
                <span
                  className="min-w-0 flex-1 truncate"
                  title={`${p.using ?? ""} ${p.check ?? ""}`}
                >
                  {p.name} · {p.cmd} · {p.roles}
                </span>
                {canWrite && (
                  <ConfirmButton
                    size="sm"
                    label="×"
                    confirmLabel={t("db.dropPolicyConfirm")}
                    onConfirm={() => doDropPolicy(p.name)}
                  />
                )}
              </li>
            ))}
          </ul>
          </div>
        )}
      </Section>

      <Section
        title={t("db.indexes")}
        description={t("db.indexesDesc", { n: indexes.length })}
        actions={
          canWrite ? (
            <CreateIndexDialog app={app} table={schema.table} columns={colNames} onCreated={afterChange} />
          ) : undefined
        }
      >
        {indexes.length === 0 ? (
          <p className="font-mono text-[11px] text-muted-foreground/70">{t("db.noIndexes")}</p>
        ) : (
          <ul className="space-y-1">
            {indexes.map((ix) => (
              <li
                key={ix.name}
                className="flex items-center gap-2 border border-border/60 bg-card/40 px-2 py-1.5 font-mono text-[11px]"
              >
                <span className="min-w-0 flex-1 truncate" title={ix.def}>
                  {ix.name}
                  <span className="ml-2 text-muted-foreground">{ix.def}</span>
                </span>
                {ix.primary && (
                  <span className="border border-signal/50 px-1 text-[9px] text-signal">PK</span>
                )}
                {ix.unique && !ix.primary && (
                  <span className="border border-border px-1 text-[9px] text-muted-foreground">UNIQUE</span>
                )}
                {canWrite && !ix.primary && (
                  <ConfirmButton
                    size="sm"
                    label="×"
                    confirmLabel={t("db.dropIndexConfirm")}
                    onConfirm={() => doDropIndex(ix.name)}
                  />
                )}
              </li>
            ))}
          </ul>
        )}
      </Section>

      <Section
        title={t("db.constraints")}
        description={t("db.constraintsDesc", { n: constraints.length })}
      >
        {constraints.length === 0 ? (
          <p className="font-mono text-[11px] text-muted-foreground/70">{t("db.noConstraints")}</p>
        ) : (
          <ul className="space-y-1">
            {constraints.map((c) => (
              <li
                key={c.name}
                className="flex items-center gap-2 border border-border/60 bg-card/40 px-2 py-1.5 font-mono text-[11px]"
              >
                <span className="border border-border px-1 text-[9px] uppercase text-muted-foreground">
                  {c.kind}
                </span>
                <span className="min-w-0 flex-1 truncate" title={c.def}>
                  {c.name}
                  <span className="ml-2 text-muted-foreground">{c.def}</span>
                </span>
              </li>
            ))}
          </ul>
        )}
      </Section>
    </div>
  );
}
