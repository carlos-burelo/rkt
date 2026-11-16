import { describe, expect, it } from "vitest";
import { isReadOnly, tokenizeSql, TEMPLATES } from "@/features/databases/sql";

describe("tokenizeSql", () => {
  it("marca keywords sin importar mayúsculas", () => {
    const kinds = tokenizeSql("SeLeCT id FROM t").map((t) => t.kind);
    expect(kinds).toContain("kw");
    expect(tokenizeSql("select").map((t) => t.kind)).toEqual(["kw"]);
  });

  it("marca strings simples y comilladas", () => {
    expect(tokenizeSql("'hola'").map((t) => t.kind)).toEqual(["str"]);
    expect(tokenizeSql("'it''s'").map((t) => t.kind)).toEqual(["str"]);
  });

  it("marca comentarios de línea y bloque", () => {
    expect(tokenizeSql("-- hola\nselect").map((t) => t.kind)).toEqual(["comment", "plain", "kw"]);
    expect(tokenizeSql("/* x */select").map((t) => t.kind)).toEqual(["comment", "kw"]);
  });

  it("marca números y funciones", () => {
    expect(tokenizeSql("42").map((t) => t.kind)).toEqual(["num"]);
    expect(tokenizeSql("count(*)").map((t) => t.kind)).toEqual(["kw", "plain"]);
    expect(tokenizeSql("gen_random_uuid()").map((t) => t.kind)).toEqual(["fn", "plain"]);
  });

  it("reconstruye el fuente exacto", () => {
    const src = "select id, 'x' -- c\nfrom t where n = 42 and f(count) > 0;";
    expect(tokenizeSql(src).map((t) => t.text).join("")).toBe(src);
  });
});

describe("isReadOnly", () => {
  it("acepta lecturas con espacios y paréntesis", () => {
    expect(isReadOnly("  select 1")).toBe(true);
    expect(isReadOnly("WITH x AS (SELECT 1) SELECT * FROM x")).toBe(true);
    expect(isReadOnly("explain select 1")).toBe(true);
  });

  it("rechaza mutaciones", () => {
    expect(isReadOnly("CREATE TABLE t (id int)")).toBe(false);
    expect(isReadOnly("insert into t values (1)")).toBe(false);
    expect(isReadOnly("")).toBe(false);
  });
});

describe("TEMPLATES", () => {
  it("son de una sola sentencia válida a simple vista", () => {
    expect(TEMPLATES.length).toBeGreaterThan(0);
    for (const tpl of TEMPLATES) {
      expect(tpl.sql.trim().length).toBeGreaterThan(0);
    }
  });
});
