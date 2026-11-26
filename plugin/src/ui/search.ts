// Panel search rules, kept out of App.svelte so they can be tested.

import { t } from "./strings";

/** Where a go_to_nodes request came from. The core echoes it in the reply. */
export type SearchSource = "search" | "list";

/** Enter submits, except the Enter that confirms an input method (Telex, IME) composition. */
export function isSubmitKey(event: { key: string; isComposing: boolean }): boolean {
  return event.key === "Enter" && !event.isComposing;
}

/**
 * What the panel shows for a go_to_nodes reply. A list click already shows its
 * result by moving the view, so only its error is reported, and not under the
 * search box, which holds the person's own text.
 */
export function searchOutcome(reply: {
  from?: SearchSource;
  error?: string;
  count?: number;
  pageName?: string;
}): { line: string; failed: boolean } | { notify: string } | null {
  if (reply.from === "list") return reply.error ? { notify: reply.error } : null;
  if (reply.error) return { line: reply.error, failed: true };
  return { line: t.searchWent(reply.count ?? 0, reply.pageName ?? ""), failed: false };
}
