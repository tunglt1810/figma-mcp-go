import { describe, expect, test } from "bun:test";
import {
  ActivityEntry,
  MAX_ENTRIES,
  finishEntry,
  formatDuration,
  formatLog,
  progressEntry,
  startEntry,
} from "./activity";

const start = (log: ActivityEntry[], id: string, tool = "get_nodes_info", now = 0) =>
  startEntry(log, id, tool, now);

describe("startEntry", () => {
  test("records the tool as running, newest first", () => {
    let log = start([], "a", "get_nodes_info", 100);
    log = start(log, "b", "create_frame", 200);
    expect(log.map((e) => e.requestId)).toEqual(["b", "a"]);
    expect(log[0].tool).toBe("create_frame");
    expect(log[0].status).toBe("running");
  });

  test("caps the log", () => {
    let log: ActivityEntry[] = [];
    for (let i = 0; i < MAX_ENTRIES + 5; i++) log = start(log, `r${i}`, "get_nodes_info", i);
    expect(log.length).toBe(MAX_ENTRIES);
    expect(log[0].requestId).toBe(`r${MAX_ENTRIES + 4}`);
  });

  test("a repeated request id replaces the old entry rather than doubling it", () => {
    let log = start([], "a", "get_nodes_info", 100);
    log = start(log, "a", "get_nodes_info", 300);
    expect(log.length).toBe(1);
    expect(log[0].startedAt).toBe(300);
  });
});

describe("progressEntry", () => {
  test("attaches the latest message to the running entry", () => {
    let log = start([], "a", "search_nodes", 0);
    log = progressEntry(log, "a", "Searched Page 1");
    expect(log[0].message).toBe("Searched Page 1");
  });

  test("keeps the previous message when a progress frame carries none", () => {
    let log = start([], "a", "search_nodes", 0);
    log = progressEntry(log, "a", "Searched Page 1");
    log = progressEntry(log, "a", undefined);
    expect(log[0].message).toBe("Searched Page 1");
  });

  test("leaves a finished entry alone", () => {
    let log = start([], "a", "search_nodes", 0);
    log = finishEntry(log, "a", undefined, 50);
    log = progressEntry(log, "a", "late frame");
    expect(log[0].message).toBeUndefined();
  });
});

describe("finishEntry", () => {
  test("marks success and stamps the end", () => {
    let log = start([], "a", "get_nodes_info", 100);
    log = finishEntry(log, "a", undefined, 250);
    expect(log[0].status).toBe("ok");
    expect(log[0].endedAt).toBe(250);
  });

  test("marks failure and keeps the error text", () => {
    let log = start([], "a", "get_nodes_info", 100);
    log = finishEntry(log, "a", "Node not found: 1:2", 150);
    expect(log[0].status).toBe("error");
    expect(log[0].message).toBe("Node not found: 1:2");
  });

  test("clears a stale progress message on success", () => {
    let log = start([], "a", "search_nodes", 0);
    log = progressEntry(log, "a", "Searched Page 1");
    log = finishEntry(log, "a", undefined, 10);
    expect(log[0].message).toBeUndefined();
  });

  // A response whose entry was trimmed away must not invent one: with no start
  // time its duration would be meaningless.
  test("ignores a response for a request the log no longer holds", () => {
    const log = finishEntry([], "gone", undefined, 100);
    expect(log).toEqual([]);
  });
});

describe("formatDuration", () => {
  const entry = (startedAt: number, endedAt?: number): ActivityEntry => ({
    requestId: "a",
    tool: "get_nodes_info",
    startedAt,
    endedAt,
    status: "ok",
  });

  test("milliseconds under a second", () => {
    expect(formatDuration(entry(0, 250), 0)).toBe("250ms");
  });

  test("seconds with one decimal", () => {
    expect(formatDuration(entry(0, 2500), 0)).toBe("2.5s");
  });

  test("minutes and seconds past a minute", () => {
    expect(formatDuration(entry(0, 125000), 0)).toBe("2m05s");
  });

  test("counts up from now while still running", () => {
    expect(formatDuration({ ...entry(1000), status: "running", endedAt: undefined }, 1400)).toBe("400ms");
  });

  test("never reports a negative duration when the clock jumps back", () => {
    expect(formatDuration(entry(500, 200), 0)).toBe("0ms");
  });
});

describe("formatLog", () => {
  test("renders one line per entry, with errors spelled out", () => {
    let log = start([], "a", "get_nodes_info", 0);
    log = finishEntry(log, "a", "Node not found", 40);
    log = start(log, "b", "create_frame", 40);
    log = finishEntry(log, "b", undefined, 90);
    expect(formatLog(log, 90)).toBe(
      "OK  create_frame (50ms)\nERR get_nodes_info (40ms) — Node not found",
    );
  });
});
