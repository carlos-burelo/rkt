import { afterEach, describe, expect, it, vi } from "vitest";
import { useCreate } from "@/features/apps/create.store";

function gitState() {
  useCreate.getState().reset();
  useCreate.setState({
    step: 2 as const,
    origin: "git" as const,
    repo: "https://github.com/o/r.git",
    name: "r",
    command: "",
    manualRequired: false,
    advanced: false,
    plan: null,
    planBusy: false,
    tried: false,
    busy: false,
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
  useCreate.getState().reset();
});

async function flush() {
  await new Promise((r) => setTimeout(r, 20));
}

describe("create git sin GitHub App", () => {
  it("detecta el tipo y avanza al paso 3", async () => {
    gitState();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({ framework: "node", start: ["node", "s.js"], install: [], build: [] }),
      ),
    );
    useCreate.getState().next();
    // planBusy bloquea el doble click mientras detecta
    expect(useCreate.getState().planBusy).toBe(true);
    await flush();
    const s = useCreate.getState();
    expect(s.step).toBe(3);
    expect(s.plan?.framework).toBe("node");
    expect(s.manualRequired).toBe(false);
    expect(s.planBusy).toBe(false);
  });

  it("sin plan se queda y pide comando manual", async () => {
    gitState();
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse({ framework: "" })));
    useCreate.getState().next();
    await flush();
    const s = useCreate.getState();
    expect(s.step).toBe(2);
    expect(s.manualRequired).toBe(true);
    expect(s.advanced).toBe(true);
    expect(s.plan).toBeNull();
    // con comando ya pasa al 3 sin volver a detectar
    const fetchMock = vi.fn(async () => jsonResponse({ framework: "node" }));
    vi.stubGlobal("fetch", fetchMock);
    useCreate.setState({ command: "node server.js" });
    useCreate.getState().next();
    expect(fetchMock).not.toHaveBeenCalled();
    expect(useCreate.getState().step).toBe(3);
  });

  it("con comando desde el inicio no llama a detectar", async () => {
    gitState();
    const fetchMock = vi.fn(async () => jsonResponse({ framework: "node" }));
    vi.stubGlobal("fetch", fetchMock);
    useCreate.setState({ command: "node server.js" });
    useCreate.getState().next();
    await flush();
    expect(fetchMock).not.toHaveBeenCalled();
    expect(useCreate.getState().step).toBe(3);
    expect(useCreate.getState().manualRequired).toBe(false);
  });

  it("error de red cae al manual en vez de romper", async () => {
    gitState();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new TypeError("caído");
      }),
    );
    useCreate.getState().next();
    await flush();
    const s = useCreate.getState();
    expect(s.step).toBe(2);
    expect(s.manualRequired).toBe(true);
  });

  it("si cambia el repo mid-detección, descarta", async () => {
    gitState();
    let resolve!: (v: Response) => void;
    const gate = new Promise<Response>((r) => {
      resolve = r;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(() => gate),
    );
    useCreate.getState().next();
    expect(useCreate.getState().planBusy).toBe(true);
    useCreate.setState({ repo: "https://github.com/o/otro.git" });
    resolve(jsonResponse({ framework: "node" }));
    await flush();
    const s = useCreate.getState();
    expect(s.step).toBe(2);
    expect(s.plan).toBeNull();
    expect(s.planBusy).toBe(false);
    expect(s.manualRequired).toBe(false);
  });

  it("no reintenta solo al insistir con el mismo repo", async () => {
    gitState();
    const fetchMock = vi.fn(async () => jsonResponse({ framework: "" }));
    vi.stubGlobal("fetch", fetchMock);
    useCreate.getState().next();
    await flush();
    expect(useCreate.getState().manualRequired).toBe(true);
    useCreate.getState().next();
    await flush();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(useCreate.getState().step).toBe(2);
  });
});
