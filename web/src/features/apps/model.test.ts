import { describe, expect, it } from "vitest";
import { canSaveApp, normalizeDomain } from "./model";

describe("canSaveApp", () => {
  it("permite externas sin command", () => {
    expect(canSaveApp("", true)).toBe(true);
  });
  it("permite estáticas sin command", () => {
    expect(canSaveApp("", false, "C:/sitio")).toBe(true);
  });
  it("exige command en apps gestionadas", () => {
    expect(canSaveApp("  ", false)).toBe(false);
    expect(canSaveApp("node server.js", false)).toBe(true);
  });
});

describe("normalizeDomain", () => {
  it("quita esquema, path y punto final", () => {
    expect(normalizeDomain(" HTTPS://App.Localhost/path ")).toBe("app.localhost");
    expect(normalizeDomain("app.test.")).toBe("app.test");
  });
});
