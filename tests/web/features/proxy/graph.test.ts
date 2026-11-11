import { describe, expect, it } from "vitest";
import { buildLinks, groupByTarget, isWildcard, radialPos, shortTarget } from "@/features/proxy/graph";

describe("isWildcard", () => {
  it("detecta comodín DNS", () => {
    expect(isWildcard("*.ejemplo.com")).toBe(true);
    expect(isWildcard("api.ejemplo.com")).toBe(false);
    expect(isWildcard("*.localhost")).toBe(true);
  });
});

describe("shortTarget", () => {
  it("loopback se abrevia a local", () => {
    expect(shortTarget("http://127.0.0.1:3000")).toBe("local:3000");
    expect(shortTarget("http://localhost:8080/x")).toBe("local:8080");
  });

  it("host externo queda intacto", () => {
    expect(shortTarget("https://api.example.com/v1")).toBe("api.example.com");
  });
});

describe("buildLinks", () => {
  it("expande hosts y ordena por host", () => {
    const links = buildLinks([
      { hosts: ["b.test", "a.test"], target: "http://127.0.0.1:1", ok: true, tls: [] },
    ]);
    expect(links.map((l) => l.host)).toEqual(["a.test", "b.test"]);
    expect(links[0]?.short).toBe("local:1");
    expect(links[0]?.tls).toBe("off");
  });
});

describe("groupByTarget", () => {
  it("agrupa y ordena por número de hosts", () => {
    const links = buildLinks([
      { hosts: ["solo.test"], target: "http://127.0.0.1:1", ok: true, tls: [] },
      { hosts: ["a.test", "b.test"], target: "http://127.0.0.1:2", ok: false, tls: [] },
    ]);
    const groups = groupByTarget(links);
    expect(groups.map((g) => g.target)).toEqual(["http://127.0.0.1:2", "http://127.0.0.1:1"]);
  });
});

describe("radialPos", () => {
  it("un solo host va a la derecha", () => {
    expect(radialPos(0, 1)).toEqual({ x: 0.82, y: 0.5 });
  });

  it("n hosts quedan dentro del cuadrado unidad", () => {
    for (let n = 2; n <= 8; n++) {
      for (let i = 0; i < n; i++) {
        const { x, y } = radialPos(i, n);
        expect(x).toBeGreaterThanOrEqual(0);
        expect(x).toBeLessThanOrEqual(1);
        expect(y).toBeGreaterThanOrEqual(0);
        expect(y).toBeLessThanOrEqual(1);
      }
    }
  });
});
