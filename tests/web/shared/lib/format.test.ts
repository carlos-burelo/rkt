import { describe, expect, it } from "vitest";
import { bytes, uptime } from "@/shared/lib/format";

describe("bytes", () => {
  it("cero y negativos son 0 B", () => {
    expect(bytes(0)).toBe("0 B");
    expect(bytes(-5)).toBe("0 B");
  });

  it("escala por 1024 con un decimal bajo 10", () => {
    expect(bytes(512)).toBe("512 B");
    expect(bytes(1024)).toBe("1.0 KB");
    expect(bytes(2048)).toBe("2.0 KB");
    expect(bytes(1048576)).toBe("1.0 MB");
  });

  it("redondea a partir de 10", () => {
    expect(bytes(12 * 1024)).toBe("12 KB");
  });
});

describe("uptime", () => {
  it("cero es resguardo", () => {
    expect(uptime(0)).toBe("—");
  });

  it("días, horas, minutos y segundos", () => {
    expect(uptime(90061)).toBe("1d 1h");
    expect(uptime(3661)).toBe("1h 1m");
    expect(uptime(61)).toBe("1m");
    expect(uptime(45)).toBe("45s");
  });
});
