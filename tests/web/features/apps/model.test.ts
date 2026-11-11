import { describe, expect, it } from "vitest";
import { canSaveApp, normalizeDomain, stackOfApp } from "@/features/apps/model";

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

describe("stackOfApp", () => {
  it("externa y estática van primero", () => {
    expect(stackOfApp({ external: true, command: "node server.js" })).toBe("external");
    expect(stackOfApp({ static: "dist", command: "" })).toBe("static");
  });
  it("detecta frameworks por command/args", () => {
    expect(stackOfApp({ command: "pnpm", args: ["exec", "next", "start", "-H", "127.0.0.1"] })).toBe("next");
    expect(stackOfApp({ command: "pnpm", args: ["exec", "vite", "--host", "127.0.0.1"] })).toBe("vite");
    expect(stackOfApp({ command: "python", args: ["-u", "app.py"] })).toBe("python");
  });
  it("detecta el runtime por pasos de build", () => {
    expect(stackOfApp({ command: "C:/rkt/apps/api/api.exe", deploy: { build: ["go build ./cmd/api"] } })).toBe("go");
    expect(stackOfApp({ command: "node server.js", deploy: { build: ["pnpm install", "pnpm build"] } })).toBe("node");
  });
  it("detecta php, ruby, dotnet y deno", () => {
    expect(stackOfApp({ command: "php", args: ["artisan", "serve", "--port", "$PORT"] })).toBe("php");
    expect(stackOfApp({ command: "bundle", args: ["exec", "puma", "-b", "tcp://127.0.0.1:$PORT"] })).toBe("ruby");
    expect(stackOfApp({ command: "dotnet", args: ["./publish/app.dll", "--urls", "http://127.0.0.1:1"] })).toBe("dotnet");
    expect(stackOfApp({ command: "deno", args: ["serve", "--port", "$PORT", "main.ts"] })).toBe("deno");
  });
  it("no confunde server.js con el paquete serve", () => {
    expect(stackOfApp({ command: "node server.js" })).toBe("node");
  });
  it("desconocido no inventa", () => {
    expect(stackOfApp({ command: "C:/rkt/apps/legado/legado.exe" })).toBe("unknown");
    expect(stackOfApp({})).toBe("unknown");
  });
});
