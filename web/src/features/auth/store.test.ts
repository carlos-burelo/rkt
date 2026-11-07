import { describe, expect, it } from "vitest";
import { useAuth } from "./store";

describe("auth store", () => {
  it("setWho cierra setupRequired al completar setup/login", () => {
    useAuth.setState({ setupRequired: true, authed: false, who: null, ready: true });
    useAuth.getState().setWho({ user: "admin", role: "admin" });
    const s = useAuth.getState();
    expect(s.setupRequired).toBe(false);
    expect(s.authed).toBe(true);
    expect(s.isAdmin).toBe(true);
  });

  it("logout no reactiva setup", () => {
    useAuth.setState({ setupRequired: false, authed: true });
    useAuth.getState().setWho(null);
    expect(useAuth.getState().setupRequired).toBe(false);
    expect(useAuth.getState().authed).toBe(false);
  });
});
