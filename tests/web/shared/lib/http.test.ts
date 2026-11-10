import { describe, expect, it } from "vitest";
import { nextDelay, POLL_BASE_MS, POLL_MAX_MS } from "@/shared/lib/http";

describe("nextDelay", () => {
  it("base 5s en el primer fallo", () => {
    expect(nextDelay(0)).toBe(POLL_BASE_MS);
    expect(nextDelay(1)).toBe(POLL_BASE_MS);
  });

  it("duplica hasta tope 30s", () => {
    expect(nextDelay(2)).toBe(10000);
    expect(nextDelay(3)).toBe(20000);
    expect(nextDelay(4)).toBe(POLL_MAX_MS);
    expect(nextDelay(99)).toBe(POLL_MAX_MS);
  });
});
