import { describe, expect, it } from "bun:test";
import { isSubmitKey, searchOutcome } from "./search";

describe("isSubmitKey", () => {
  it("submits on Enter", () => {
    expect(isSubmitKey({ key: "Enter", isComposing: false })).toBe(true);
  });

  it("ignores the Enter that confirms an input method composition", () => {
    expect(isSubmitKey({ key: "Enter", isComposing: true })).toBe(false);
  });

  it("ignores other keys", () => {
    expect(isSubmitKey({ key: "a", isComposing: false })).toBe(false);
  });
});

describe("searchOutcome", () => {
  it("shows a search success under the box", () => {
    expect(searchOutcome({ from: "search", count: 2, pageName: "Checkout" }))
      .toEqual({ line: "Went to 2 node(s) · page Checkout", failed: false });
  });

  it("shows a search error under the box", () => {
    expect(searchOutcome({ from: "search", error: "Node not found: 9:9" }))
      .toEqual({ line: "Node not found: 9:9", failed: true });
  });

  it("leaves the search line alone after a list click that worked", () => {
    expect(searchOutcome({ from: "list", count: 1, pageName: "Checkout" })).toBeNull();
  });

  it("reports a list click error as a Figma notification", () => {
    expect(searchOutcome({ from: "list", error: "Node not found: 9:9" }))
      .toEqual({ notify: "Node not found: 9:9" });
  });
});
