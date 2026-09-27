import { describe, expect, it } from "bun:test";
import { t } from "./i18n";

// The checks that used to live here compared the English table with the
// Vietnamese one. With a single table there is nothing to compare. What still
// needs checking is that no key has a blank label, which shows up as an
// invisible control instead of an obvious mistake.
describe("the string table", () => {
  it("leaves no string empty", () => {
    for (const [key, value] of Object.entries(t)) {
      if (typeof value === "string") expect(value.length, key).toBeGreaterThan(0);
    }
  });
});
