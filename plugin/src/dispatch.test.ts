import { describe, it, expect } from "bun:test";
import { mergeHandlers } from "./dispatch";
import { readHandlers } from "./read-handlers";
import { writeHandlers } from "./write-handlers";

// The modules used to be chained, so two of them claiming the same request name
// meant whichever came first in the chain quietly won and the other was dead
// code. Merging the maps makes that a thrown error instead.
describe("mergeHandlers", () => {
  const noop = async () => null;

  it("merges disjoint maps", () => {
    const merged = mergeHandlers({ a: noop }, { b: noop });
    expect(Object.keys(merged).sort()).toEqual(["a", "b"]);
  });

  it("throws on a duplicate name, naming it", () => {
    expect(() => mergeHandlers({ get_node: noop }, { get_node: noop }))
      .toThrow(/Duplicate plugin handler for request type: get_node/);
  });
});

// Building the real maps is itself the check that no two modules collide —
// importing them here would already have thrown.
describe("the real handler maps", () => {
  it("cover reads and writes without colliding", () => {
    expect(Object.keys(readHandlers).length).toBeGreaterThan(0);
    expect(Object.keys(writeHandlers).length).toBeGreaterThan(0);
    const overlap = Object.keys(readHandlers).filter(k => k in writeHandlers);
    expect(overlap).toEqual([]);
  });

  it("answers in one lookup, not by walking every module", () => {
    // A write request used to traverse three read switches before reaching its
    // own module. Now it is a property access.
    expect(writeHandlers["set_paint"]).toBeTypeOf("function");
    expect(readHandlers["get_node"]).toBeTypeOf("function");
    expect(writeHandlers["get_node"]).toBeUndefined();
  });
});
