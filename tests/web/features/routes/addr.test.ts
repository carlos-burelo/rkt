import { describe, expect, it } from "vitest";
import { routeMode, routeAddr } from "@/features/routes/RouteParts";

describe("routeMode", () => {
  it("dominio por defecto", () => {
    expect(routeMode({ hosts: ["a.test"], target: "http://127.0.0.1:1" })).toBe("host");
  });
  it("path sin hosts", () => {
    expect(routeMode({ hosts: [], target: "http://127.0.0.1:1", path: "/app" })).toBe("path");
  });
});

describe("routeAddr", () => {
  it("dominio pelado", () => {
    expect(routeAddr({ hosts: ["app.midominio.com"], target: "x" })).toBe("app.midominio.com");
  });
  it("host + path", () => {
    expect(routeAddr({ hosts: ["app.midominio.com"], target: "x", path: "/panel" })).toBe(
      "app.midominio.com/panel",
    );
  });
  it("path sin host marca comodín", () => {
    expect(routeAddr({ hosts: [], target: "x", path: "/app" })).toBe("∗/app");
  });
});
