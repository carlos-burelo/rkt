import { describe, expect, it } from "vitest";
import { routeMode, routeAddr } from "@/features/routes/RouteParts";

describe("routeMode", () => {
  it("dominio por defecto", () => {
    expect(routeMode({ hosts: ["a.test"], target: "http://127.0.0.1:1" })).toBe("host");
  });
  it("puerto manda sobre path", () => {
    expect(routeMode({ hosts: [], target: "http://127.0.0.1:1", path: "/app", port: 3000 })).toBe("port");
  });
  it("path sin puerto", () => {
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
  it("puerto con IP", () => {
    expect(routeAddr({ hosts: ["192.0.2.10"], target: "x", port: 3000 })).toBe("192.0.2.10:3000");
  });
  it("puerto sin host", () => {
    expect(routeAddr({ hosts: [], target: "x", port: 3000 })).toBe("∗:3000");
  });
});
