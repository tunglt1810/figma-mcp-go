import { describe, expect, it } from "bun:test";
import { t } from "./i18n";

// The parity checks that used to live here compared the English table against
// the Vietnamese one; with a single table there is nothing to compare. What
// still needs guarding is that no key is a blank label, which renders as an
// invisible control rather than as an obvious mistake.
describe("the string table", () => {
  it("leaves no string empty", () => {
    for (const [key, value] of Object.entries(t)) {
      if (typeof value === "string") expect(value.length, key).toBeGreaterThan(0);
    }
  });
});
