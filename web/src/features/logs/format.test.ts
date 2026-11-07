import { describe, expect, it } from "vitest";
import { accessSummary, groupParsed, parseLogLine, rejectedSummary, streamOf } from "./format";

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

describe("streamOf", () => {
  it("clasifica accesos, rechazados y sistema", () => {
    const access = parseLogLine(
      'time=2026-11-07T11:09:23Z level=INFO msg=proxy host=app1.coatl.in method=GET path=/ target=127.0.0.1:3000 dur=9.2ms',
    );
    const rejected = parseLogLine(
      "time=2026-11-07T10:40:14Z level=WARN msg=\"sin ruta\" host=azenv.net remote=160.119.71.168:41552 path=/",
    );
    const system = parseLogLine("time=2026-11-07T11:10:33Z level=INFO msg=\"config recargada\" routes=1 apps=1");
    expect(streamOf(access, "")).toBe("access");
    expect(streamOf(rejected, "")).toBe("rejected");
    expect(streamOf(system, "")).toBe("system");
    expect(streamOf(access, "app1")).toBe("app");
  });
});

describe("summaries", () => {
  it("resume accesos como método path → target · dur", () => {
    const p = parseLogLine(
      "level=INFO msg=proxy host=app1.coatl.in method=GET path=/ target=127.0.0.1:3000 dur=9.188ms",
    );
    expect(accessSummary(p)).toBe("GET / → 127.0.0.1:3000 · 9.188ms");
  });

  it("resume rechazos como host path ← remote", () => {
    const p = parseLogLine('level=WARN msg="sin ruta" host=azenv.net remote=160.119.71.168:41552 path=/');
    expect(rejectedSummary(p)).toBe("azenv.net / ← 160.119.71.168:41552");
  });

  it("devuelve null sin campos suficientes", () => {
    expect(accessSummary(parseLogLine("level=INFO msg=proxy"))).toBeNull();
    expect(rejectedSummary(parseLogLine("level=WARN msg=hola"))).toBeNull();
  });
});
