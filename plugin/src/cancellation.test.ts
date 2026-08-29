import { describe, expect, it, beforeEach } from "bun:test";
import {
  clearCancelled,
  isCancelled,
  markCancelled,
  resetCancellations,
  throwIfCancelled,
} from "./cancellation";

beforeEach(() => resetCancellations());

describe("cancellation", () => {
  it("remembers a cancelled request", () => {
    markCancelled("r1");
    expect(isCancelled("r1")).toBe(true);
    expect(isCancelled("r2")).toBe(false);
  });

  it("treats a missing id as not cancelled", () => {
    expect(isCancelled(undefined)).toBe(false);
    expect(isCancelled("")).toBe(false);
  });

  it("ignores an empty id rather than storing one", () => {
    markCancelled("");
    expect(isCancelled("")).toBe(false);
  });

  it("forgets a request once it is over", () => {
    markCancelled("r1");
    clearCancelled("r1");
    expect(isCancelled("r1")).toBe(false);
  });

  it("throws only for a cancelled request", () => {
    markCancelled("r1");
    expect(() => throwIfCancelled("r1")).toThrow("Request cancelled");
    expect(() => throwIfCancelled("r2")).not.toThrow();
  });

  // A cancel for a request that already finished is never cleared, so the set
  // has to bound itself or it grows for the life of the session.
  it("evicts the oldest ids past its cap", () => {
    for (let i = 0; i < 300; i++) markCancelled(`r${i}`);
    expect(isCancelled("r0")).toBe(false);
    expect(isCancelled("r299")).toBe(true);
  });
});
