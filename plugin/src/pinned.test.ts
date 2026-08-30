import { beforeEach, describe, expect, it } from "bun:test";
import { clearPinned, getPinned, setPinned } from "./pinned";

beforeEach(() => clearPinned());

describe("the pinned context set", () => {
  it("starts empty", () => {
    expect(getPinned()).toEqual([]);
  });

  it("replaces the set rather than adding to it", () => {
    setPinned(["1:1", "1:2"]);
    setPinned(["2:1"]);
    expect(getPinned()).toEqual(["2:1"]);
  });

  // A designer shift-clicking the same node twice should not make it count twice.
  it("drops duplicates and blanks", () => {
    setPinned(["1:1", "1:1", "  ", "", "1:2", 7, null]);
    expect(getPinned()).toEqual(["1:1", "1:2"]);
  });

  it("hands back a copy, so a caller cannot mutate the pin", () => {
    setPinned(["1:1"]);
    getPinned().push("1:2");
    expect(getPinned()).toEqual(["1:1"]);
  });

  it("treats anything that is not an array as an empty pin", () => {
    setPinned(["1:1"]);
    setPinned("1:2" as any);
    expect(getPinned()).toEqual([]);
  });
});
