import { beforeEach, describe, expect, it, vi, afterEach } from "vitest";
import { subscribeFleet } from "./events";
import { useFleetBus } from "../../features/overview/fleet.store";

type Listener = (e: { data: string }) => void;

class FakeES {
  static instances: FakeES[] = [];
  url: string;
  listeners = new Map<string, Listener[]>();
  onerror: (() => void) | null = null;
  closed = false;
  constructor(url: string) {
    this.url = url;
    FakeES.instances.push(this);
  }
  addEventListener(t: string, cb: Listener) {
    const arr = this.listeners.get(t) ?? [];
    arr.push(cb);
    this.listeners.set(t, arr);
  }
  close() {
    this.closed = true;
  }
  emit(t: string, data: unknown) {
    for (const cb of this.listeners.get(t) ?? []) cb({ data: JSON.stringify(data) });
  }
  fail() {
    this.onerror?.();
  }
}

const fetchMock = vi.fn(async () => new Response("{}", { status: 200 }));

function resetStore() {
  useFleetBus.setState({ status: null, deps: [], evs: [], metas: [], seq: 0, conn: "retrying", loading: true, seen: false });
}

beforeEach(() => {
  FakeES.instances = [];
  resetStore();
  fetchMock.mockReset();
  fetchMock.mockImplementation(async () => new Response("{}", { status: 200 }));
  vi.stubGlobal("EventSource", FakeES);
  vi.stubGlobal("fetch", fetchMock);
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const STATUS = { version: "t", routes: 1, apps: [{ name: "a", running: true, restarts: 0 }] };

describe("fleet bus", () => {
  it("snapshot al conectar puebla el store", async () => {
    const unsub = subscribeFleet();
    try {
      await vi.advanceTimersByTimeAsync(0);
      expect(FakeES.instances).toHaveLength(1);
      const es = FakeES.instances[0];
      expect(es.url).toContain("cursor=0");
      es.emit("hello", { seq: 5, resync: true });
      es.emit("snapshot", { seq: 0, topic: "status", data: STATUS });
      es.emit("snapshot", { seq: 0, topic: "deployments", data: [{ id: 1 }] });
      const s = useFleetBus.getState();
      expect(s.status?.apps).toHaveLength(1);
      expect(s.deps).toHaveLength(1);
      expect(s.conn).toBe("live");
      expect(s.loading).toBe(false);
    } finally {
      unsub();
    }
  });

  it("live reemplaza y avanza seq; duplicados se ignoran", async () => {
    const unsub = subscribeFleet();
    try {
      await vi.advanceTimersByTimeAsync(0);
      const es = FakeES.instances[0];
      es.emit("hello", { seq: 7, resync: false });
      es.emit("status", { seq: 8, topic: "status", data: STATUS });
      es.emit("status", { seq: 8, topic: "status", data: { version: "x", routes: 0, apps: [] } });
      const s = useFleetBus.getState();
      expect(s.seq).toBe(8);
      expect(s.status?.version).toBe("t");
      es.emit("status", { seq: 9, topic: "status", data: { version: "n", routes: 0, apps: [] } });
      expect(useFleetBus.getState().status?.version).toBe("n");
    } finally {
      unsub();
    }
  });

  it("error reconecta con backoff y cierra al desuscribir", async () => {
    const unsub = subscribeFleet();
    try {
      await vi.advanceTimersByTimeAsync(0);
      const es = FakeES.instances[0];
      es.emit("hello", { seq: 1, resync: true });
      es.fail();
      expect(useFleetBus.getState().conn).toBe("retrying");
      // backoff nextDelay(1) = 5s, con probe previo
      await vi.advanceTimersByTimeAsync(5000);
      expect(FakeES.instances).toHaveLength(2);
      expect(FakeES.instances[1].closed).toBe(false);
    } finally {
      unsub();
    }
    expect(FakeES.instances.every((e) => e.closed)).toBe(true);
  });

  it("sin backend no abre SSE: offline y sin esqueletos eternos", async () => {
    fetchMock.mockRejectedValue(new Error("down"));
    const unsub = subscribeFleet();
    try {
      await vi.advanceTimersByTimeAsync(0);
      expect(FakeES.instances).toHaveLength(0);
      expect(useFleetBus.getState().conn).toBe("retrying");
      await vi.advanceTimersByTimeAsync(5000);
      expect(useFleetBus.getState().conn).toBe("retrying");
      await vi.advanceTimersByTimeAsync(10000);
      const s = useFleetBus.getState();
      expect(s.conn).toBe("offline");
      expect(s.loading).toBe(false);
      expect(FakeES.instances).toHaveLength(0);
    } finally {
      unsub();
    }
  });
});
