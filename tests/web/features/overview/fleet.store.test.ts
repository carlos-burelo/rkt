import { beforeEach, describe, expect, it } from "vitest";
import { applyFleetEvent, useFleetBus } from "@/features/overview/fleet.store";

function reset() {
  useFleetBus.setState({ status: null, deps: [], evs: [], metas: [], seq: 0, conn: "retrying", loading: true });
}

beforeEach(reset);

describe("fleet store", () => {
  it("snapshots reemplazan por tópico", () => {
    const s = useFleetBus.getState();
    s.applySnapshot("status", { version: "t", routes: 1, apps: [{ name: "a", running: true, restarts: 0 }] });
    s.applySnapshot("deployments", [{ id: 3 }]);
    s.applySnapshot("metas", [{ name: "a" }]);
    const st = useFleetBus.getState();
    expect(st.status?.version).toBe("t");
    expect(st.deps).toHaveLength(1);
    expect(st.metas).toHaveLength(1);
    expect(st.loading).toBe(false);
  });

  it("tópico desconocido o forma rota no rompe", () => {
    const s = useFleetBus.getState();
    s.applySnapshot("nope", { x: 1 });
    s.applySnapshot("status", { version: "x" });
    const st = useFleetBus.getState();
    expect(st.status).toBeNull();
    expect(st.loading).toBe(true);
  });

  it("live filtra duplicados y viejos", () => {
    applyFleetEvent(5, "events", [{ id: 1 }]);
    applyFleetEvent(5, "events", [{ id: 2 }]);
    applyFleetEvent(3, "events", [{ id: 3 }]);
    const st = useFleetBus.getState();
    expect(st.seq).toBe(5);
    expect(st.evs).toEqual([{ id: 1 }]);
  });
});
