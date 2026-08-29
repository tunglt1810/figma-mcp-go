import { describe, expect, it, beforeEach } from "bun:test";
import { enqueueWrite, resetWriteQueue } from "./write-queue";

beforeEach(() => resetWriteQueue());

const deferred = <T>() => {
  let resolve!: (value: T) => void;
  let reject!: (error: any) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
};

describe("enqueueWrite", () => {
  it("returns the work's value", async () => {
    expect(await enqueueWrite(async () => "done")).toBe("done");
  });

  it("propagates a failure to its own caller", async () => {
    expect(enqueueWrite(async () => { throw new Error("boom"); })).rejects.toThrow("boom");
  });

  it("runs one at a time, in the order queued", async () => {
    const order: string[] = [];
    const first = deferred<void>();
    const second = deferred<void>();

    const a = enqueueWrite(async () => { order.push("a:start"); await first.promise; order.push("a:end"); });
    const b = enqueueWrite(async () => { order.push("b:start"); await second.promise; order.push("b:end"); });

    // b must not have started while a is still in flight.
    await Promise.resolve();
    expect(order).toEqual(["a:start"]);

    first.resolve();
    await a;
    await Promise.resolve();
    expect(order).toEqual(["a:start", "a:end", "b:start"]);

    second.resolve();
    await b;
    expect(order).toEqual(["a:start", "a:end", "b:start", "b:end"]);
  });

  // A rejected predecessor has already reported its own failure; it must not
  // take the next request down with it.
  it("keeps serving after a failure", async () => {
    const failed = enqueueWrite(async () => { throw new Error("boom"); });
    await failed.catch(() => {});
    expect(await enqueueWrite(async () => "still here")).toBe("still here");
  });

  it("does not let a failure reach the next caller", async () => {
    enqueueWrite(async () => { throw new Error("boom"); }).catch(() => {});
    await expect(enqueueWrite(async () => "fine")).resolves.toBe("fine");
  });

  it("keeps order across a failure", async () => {
    const order: string[] = [];
    enqueueWrite(async () => { order.push("a"); throw new Error("boom"); }).catch(() => {});
    await enqueueWrite(async () => { order.push("b"); });
    expect(order).toEqual(["a", "b"]);
  });
});
