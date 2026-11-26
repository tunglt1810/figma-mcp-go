import { describe, expect, it } from "bun:test";
import { parseNodeIds } from "./node-ids";

describe("parseNodeIds", () => {
  it("accepts a bare id", () => {
    expect(parseNodeIds("12:34")).toEqual({ ids: ["12:34"], invalid: [] });
  });

  it("accepts the URL form with a dash", () => {
    expect(parseNodeIds("12-34").ids).toEqual(["12:34"]);
  });

  it("takes node-id from a Figma link", () => {
    expect(parseNodeIds("https://www.figma.com/design/AbC/Shop?node-id=12-34&t=x").ids).toEqual(["12:34"]);
  });

  it("decodes an encoded node-id", () => {
    expect(parseNodeIds("https://www.figma.com/design/AbC/Shop?node-id=12%3A34").ids).toEqual(["12:34"]);
  });

  it("keeps an instance sublayer id whole", () => {
    expect(parseNodeIds("I12-34;56-78").ids).toEqual(["I12:34;56:78"]);
  });

  it("splits on commas and whitespace, drops duplicates and empty tokens", () => {
    expect(parseNodeIds("12:34, 12-35\n12:34,").ids).toEqual(["12:34", "12:35"]);
  });

  it("reports tokens that are not ids", () => {
    expect(parseNodeIds("12:34 abc 12:")).toEqual({ ids: ["12:34"], invalid: ["abc", "12:"] });
  });

  it("reports a link without node-id", () => {
    const url = "https://www.figma.com/design/AbC/Shop";
    expect(parseNodeIds(url)).toEqual({ ids: [], invalid: [url] });
  });

  it("returns nothing for blank input", () => {
    expect(parseNodeIds("   ")).toEqual({ ids: [], invalid: [] });
  });
});
