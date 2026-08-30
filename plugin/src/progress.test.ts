import { describe, expect, it, beforeEach } from "bun:test";
import { clampProgress, reportProgress, stepProgress } from "./progress";

let posted: any[];

beforeEach(() => {
  posted = [];
  (globalThis as any).figma = {
    ui: { postMessage: (msg: any) => posted.push(msg) },
  };
});

describe("clampProgress", () => {
  it("keeps progress inside 1–99", () => {
    expect(clampProgress(0)).toBe(1);
    expect(clampProgress(-5)).toBe(1);
    expect(clampProgress(100)).toBe(99);
    expect(clampProgress(42.6)).toBe(43);
  });
});

describe("stepProgress", () => {
  it("scales a step count across the range", () => {
    expect(stepProgress(0, 4)).toBe(1);
    expect(stepProgress(2, 4)).toBe(50);
  });

  // The response is what says the work finished, not a progress message.
  it("never reports 100 on the last step", () => {
    expect(stepProgress(4, 4)).toBe(99);
  });

  it("survives a zero total rather than dividing by it", () => {
    expect(stepProgress(0, 0)).toBe(1);
  });
});

describe("reportProgress", () => {
  it("posts the message the server extends a timeout on", async () => {
    await reportProgress("r1", 50, "halfway");
    expect(posted).toEqual([
      { type: "progress_update", requestId: "r1", progress: 50, message: "halfway" },
    ]);
  });
});
