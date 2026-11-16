// SQL del Studio: resaltado propio (sin dependencias), heurística de
// lectura (espejo de pg.isReadOnly: solo cambia el gating del botón, el
// servidor decide) y plantillas de inicio rápido.

export type TokKind = "kw" | "str" | "comment" | "num" | "fn" | "plain";

export interface Tok {
  text: string;
  kind: TokKind;
}

const KEYWORDS = new Set(
  `select from where with as and or not null true false insert into values
   update set delete create table alter add column drop index constraint
   primary key foreign references unique check default on cascade restrict
   enable row level security policy for using to join left right inner outer
   full on group by order having limit offset distinct count sum avg min max
   references returning if exists like ilike in between is begin commit
   rollback transaction explain show describe table values`.split(/\s+/),
);

// isReadOnly espejo del backend: SELECT/WITH/TABLE/VALUES/EXPLAIN/SHOW/DESCRIBE.
export function isReadOnly(sql: string): boolean {
  const w = sql.trimStart().toUpperCase();
  for (const p of ["SELECT", "WITH", "TABLE", "VALUES", "EXPLAIN", "SHOW", "DESCRIBE"]) {
    if (w === p || w.startsWith(p + " ") || w.startsWith(p + "(")) return true;
  }
  return false;
}

const RE =
  /(--[^\n]*)|(\/\*[\s\S]*?(?:\*\/|$))|('(?:[^']|'')*(?:'|$))|("[^"\n]*(?:"|$))|\b(\d+(?:\.\d+)?)\b|([A-Za-z_][A-Za-z0-9_$]*)/g;

// tokenizeSql parte el fuente en tokens para colorear. Puro y testeable.
export function tokenizeSql(src: string): Tok[] {
  const out: Tok[] = [];
  let last = 0;
  RE.lastIndex = 0;
  for (;;) {
    const m = RE.exec(src);
    if (!m) break;
    if (m.index > last) out.push({ text: src.slice(last, m.index), kind: "plain" });
    const [full, line, block, str, ident, num, word] = m;
    if (line !== undefined || block !== undefined) out.push({ text: full, kind: "comment" });
    else if (str !== undefined || ident !== undefined) out.push({ text: full, kind: "str" });
    else if (num !== undefined) out.push({ text: full, kind: "num" });
    else if (word !== undefined) {
      if (KEYWORDS.has(word.toLowerCase())) out.push({ text: full, kind: "kw" });
      else {
        const after = src.slice(m.index + full.length);
        out.push({ text: full, kind: /^\s*\(/.test(after) ? "fn" : "plain" });
      }
    } else out.push({ text: full, kind: "plain" });
    last = m.index + full.length;
  }
  if (last < src.length) out.push({ text: src.slice(last), kind: "plain" });
  return out;
}

export interface Template {
  name: string;
  sql: string;
}

// TEMPLATES inicio rápido: el esquema lo crea SQL, no un editor visual.
export const TEMPLATES: Template[] = [
  {
    name: "tabla con uuid",
    sql: `create table perfiles (
  id uuid primary key default gen_random_uuid(),
  email text unique not null,
  nombre text,
  creado_en timestamptz not null default now()
);`,
  },
  {
    name: "índice",
    sql: `create index perfiles_email_idx on perfiles (email);`,
  },
  {
    name: "columna",
    sql: `alter table perfiles
  add column apodo text;`,
  },
  {
    name: "lectura pública (RLS)",
    sql: `alter table perfiles
  enable row level security;

create policy "lectura pública"
  on perfiles for select
  using (true);`,
  },
  {
    name: "dueño por auth (RLS)",
    sql: `create policy "dueños ven lo suyo"
  on perfiles for all
  using (auth.uid() = id);`,
  },
];
