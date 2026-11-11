import { describe, expect, it } from "vitest";
import { bytes, rate, tone, uptime } from "@/features/server/format";

describe("bytes del monitor", () => {
  it("NaN/Infinity son resguardo, no número", () => {
    expect(bytes(NaN)).toBe("—");
    expect(bytes(Infinity)).toBe("—");
  });

  it("delega al formato canónico", () => {
    expect(bytes(0)).toBe("0 B");
    expect(bytes(2048)).toBe("2.0 KB");
  });
});

describe("rate", () => {
  it("lleva /s", () => {
    expect(rate(1024)).toBe("1.0 KB/s");
  });
});

describe("uptime del monitor", () => {
  const t = (k: string, v?: Record<string, string | number>) =>
    `${k} ${Object.values(v ?? {}).join(",")}`;

  it("días, horas y minutos", () => {
    expect(uptime(90061, t)).toContain("server.uptime.days");
    expect(uptime(3661, t)).toContain("server.uptime.hours");
    expect(uptime(61, t)).toContain("server.uptime.minutes");
  });
});

describe("tone", () => {
  it("umbrales 0.7/0.9", () => {
    expect(tone(undefined)).toBe("signal");
    expect(tone(0.5)).toBe("ok");
    expect(tone(0.8)).toBe("warn");
    expect(tone(0.95)).toBe("bad");
  });
});
