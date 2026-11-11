import { describe, expect, it } from "vitest";
import { cronStatusOf, cronTone } from "@/features/cron/status";

describe("cronStatusOf", () => {
  it("running manda sobre el último estado", () => {
    expect(cronStatusOf({ running: true, last: { status: "failed" } })).toBe("running");
  });

  it("sin historial es idle", () => {
    expect(cronStatusOf({ running: false })).toBe("idle");
    expect(cronStatusOf({ running: false, last: null })).toBe("idle");
  });

  it("devuelve el último estado", () => {
    expect(cronStatusOf({ running: false, last: { status: "ok" } })).toBe("ok");
    expect(cronStatusOf({ running: false, last: { status: "failed" } })).toBe("failed");
  });
});

describe("cronTone", () => {
  it("mapea cada estado a su tono", () => {
    expect(cronTone("ok")).toBe("ok");
    expect(cronTone("running")).toBe("running");
    expect(cronTone("failed")).toBe("bad");
    expect(cronTone("timeout")).toBe("warn");
  });

  it("desconocido es stopped", () => {
    expect(cronTone("otra-cosa")).toBe("stopped");
    expect(cronTone("")).toBe("stopped");
  });
});
