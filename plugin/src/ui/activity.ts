// The panel's activity log.
//
// "AI is working…" said nothing about what was running, so a stuck or failing
// tool looked identical to a slow one and the only way to find out was the
// server's stdio log. The request payload already carries the tool name and a
// request id, so the panel can simply show them.
//
// Kept out of the component so the ring-buffer and status rules can be tested
// without mounting Svelte.

export type ActivityStatus = "running" | "ok" | "error";

export interface ActivityEntry {
  requestId: string;
  tool: string;
  startedAt: number;
  endedAt?: number;
  status: ActivityStatus;
  /** Latest progress message while running; the error text once failed. */
  message?: string;
}

/** How many entries the log keeps. The panel shows a handful; the rest is for
 * the copy-to-clipboard dump that goes into a bug report. */
export const MAX_ENTRIES = 20;

/** Record a request that has just been handed to the plugin core. */
export function startEntry(
  log: ActivityEntry[],
  requestId: string,
  tool: string,
  now: number,
): ActivityEntry[] {
  // Newest first: the panel shows the top of the list, which is where anything
  // worth looking at just happened.
  const next = [
    { requestId, tool, startedAt: now, status: "running" as ActivityStatus },
    ...log.filter((entry) => entry.requestId !== requestId),
  ];
  return next.slice(0, MAX_ENTRIES);
}

/** Attach the latest progress message to a running entry. */
export function progressEntry(
  log: ActivityEntry[],
  requestId: string,
  message: string | undefined,
): ActivityEntry[] {
  return log.map((entry) =>
    entry.requestId === requestId && entry.status === "running"
      ? { ...entry, message: message || entry.message }
      : entry,
  );
}

/**
 * Close out an entry.
 *
 * A response for a request the log never saw is ignored rather than invented:
 * it means the log was trimmed under a long run, and a synthetic entry with no
 * start time would report a nonsense duration.
 */
export function finishEntry(
  log: ActivityEntry[],
  requestId: string,
  error: string | undefined,
  now: number,
): ActivityEntry[] {
  return log.map((entry) =>
    entry.requestId === requestId && entry.status === "running"
      ? {
          ...entry,
          endedAt: now,
          status: error ? ("error" as ActivityStatus) : ("ok" as ActivityStatus),
          message: error || undefined,
        }
      : entry,
  );
}

/** Human-readable elapsed time, short enough for a 320px panel. */
export function formatDuration(entry: ActivityEntry, now: number): string {
  const end = entry.endedAt ?? now;
  const ms = Math.max(0, end - entry.startedAt);
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  const minutes = Math.floor(ms / 60000);
  const seconds = Math.round((ms % 60000) / 1000);
  return `${minutes}m${seconds.toString().padStart(2, "0")}s`;
}

/** The whole log as text, for pasting into a bug report. */
export function formatLog(log: ActivityEntry[], now: number): string {
  return log
    .map((entry) => {
      const mark = entry.status === "ok" ? "OK " : entry.status === "error" ? "ERR" : "..." ;
      const detail = entry.message ? ` — ${entry.message}` : "";
      return `${mark} ${entry.tool} (${formatDuration(entry, now)})${detail}`;
    })
    .join("\n");
}
