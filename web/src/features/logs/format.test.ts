import { describe, expect, it } from "vitest";
import { groupParsed, parseLogLine } from "./format";

describe("parseLogLine", () => {
  it("parsea logfmt con tiempo, nivel, mensaje y campos", () => {
    const p = parseLogLine("time=2026-11-02T11:00:00Z level=WARN checkout: slow query path=/api dur=412ms");
    expect(p.structured).toBe(true);
    expect(p.level).toBe("warning");
    expect(p.msg).toBe("checkout: slow query");
    expect(p.fields.map((f) => f.key)).toEqual(["path", "dur"]);
    expect(p.timestamp).toBeTypeOf("number");
  });

  it("parsea JSON", () => {
    const p = parseLogLine('{"time":"2026-11-02T11:00:00Z","level":"error","msg":"boom","status":500}');
    expect(p.level).toBe("error");
    expect(p.msg).toBe("boom");
    expect(p.fields).toEqual([{ key: "status", value: "500" }]);
  });

  it("trata texto plano como info", () => {
    const p = parseLogLine("hola mundo");
    expect(p.structured).toBe(false);
    expect(p.level).toBe("info");
    expect(p.msg).toBe("hola mundo");
  });

  it("detecta nivel por prefijo", () => {
    expect(parseLogLine("ERROR algo").level).toBe("error");
    expect(parseLogLine("WARN algo").level).toBe("warning");
  });
});

describe("groupParsed", () => {
  it("agrupa líneas consecutivas idénticas", () => {
    const lines = [
      parseLogLine("level=INFO msg=ok a=1"),
      parseLogLine("level=INFO msg=ok a=1"),
      parseLogLine("level=INFO msg=ok a=2"),
    ];
    const grouped = groupParsed(lines);
    expect(grouped).toHaveLength(2);
    expect(grouped[0].count).toBe(2);
    expect(grouped[1].count).toBe(1);
  });
});
