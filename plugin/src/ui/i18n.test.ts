import { describe, expect, it } from "bun:test";
import { LOCALES, pickLocale, strings } from "./i18n";

describe("pickLocale", () => {
  it("picks Vietnamese from any Vietnamese tag", () => {
    expect(pickLocale(["vi"])).toBe("vi");
    expect(pickLocale(["vi-VN"])).toBe("vi");
    expect(pickLocale(["VI-vn"])).toBe("vi");
  });

  it("honours the order the browser gives", () => {
    expect(pickLocale(["en-GB", "vi"])).toBe("vi");
    expect(pickLocale(["fr", "en"])).toBe("en");
  });

  // A half-translated panel is worse than an English one.
  it("falls back to English for anything else, and for nothing at all", () => {
    expect(pickLocale(["ja"])).toBe("en");
    expect(pickLocale([])).toBe("en");
    expect(pickLocale(undefined)).toBe("en");
  });
});

describe("the string tables", () => {
  // A key added to one table and forgotten in the other shows an English word
  // in the middle of a Vietnamese panel, which no test would otherwise catch.
  it("define exactly the same keys in every locale", () => {
    const keys = LOCALES.map(locale => Object.keys(strings(locale)).sort());
    for (const set of keys) expect(set).toEqual(keys[0]);
  });

  it("give the same shape for every key", () => {
    for (const key of Object.keys(strings("en")) as Array<keyof ReturnType<typeof strings>>) {
      expect(typeof strings("vi")[key]).toBe(typeof strings("en")[key]);
    }
  });

  it("leaves no string empty", () => {
    for (const locale of LOCALES) {
      for (const [key, value] of Object.entries(strings(locale))) {
        if (typeof value === "string") expect(value.length, `${locale}.${key}`).toBeGreaterThan(0);
      }
    }
  });
});
