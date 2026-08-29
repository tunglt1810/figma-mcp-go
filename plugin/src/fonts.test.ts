import { describe, expect, it } from "bun:test";
import { fontKey, loadFonts } from "./fonts";

const Inter = { family: "Inter", style: "Regular" };
const InterBold = { family: "Inter", style: "Bold" };
const Roboto = { family: "Roboto", style: "Regular" };

describe("fontKey", () => {
  it("names a font the way Figma does", () => {
    expect(fontKey(Inter)).toBe("Inter Regular");
  });
});

describe("loadFonts", () => {
  it("loads each font once, however many times it was asked for", async () => {
    const loaded: string[] = [];
    await loadFonts([Inter, Inter, InterBold], async (font) => {
      loaded.push(fontKey(font));
    });
    expect(loaded.sort()).toEqual(["Inter Bold", "Inter Regular"]);
  });

  it("does nothing for an empty list", async () => {
    let called = false;
    await loadFonts([], async () => { called = true; });
    expect(called).toBe(false);
  });

  it("skips a malformed font rather than asking Figma for it", async () => {
    const loaded: string[] = [];
    await loadFonts(
      [Inter, { family: "", style: "Regular" }, { family: "Inter" } as any, null as any],
      async (font) => { loaded.push(fontKey(font)); },
    );
    expect(loaded).toEqual(["Inter Regular"]);
  });

  // The reason this module exists: one attempt used to surface one missing
  // font, so a caller fixed them one round trip at a time.
  it("names every missing font, not just the first", async () => {
    const attempt = loadFonts([Inter, Roboto, InterBold], async (font) => {
      if (font.family !== "Inter") throw new Error("not available");
      if (font.style === "Bold") throw new Error("not available");
    });
    await expect(attempt).rejects.toThrow(/Inter Bold/);
    await expect(
      loadFonts([Roboto, InterBold], async () => { throw new Error("nope"); }),
    ).rejects.toThrow("Fonts not available in this file: Roboto Regular, Inter Bold");
  });

  it("reads naturally when only one font is missing", async () => {
    await expect(
      loadFonts([Roboto], async () => { throw new Error("nope"); }),
    ).rejects.toThrow("Font not available in this file: Roboto Regular");
  });

  it("resolves when every font loads", async () => {
    await loadFonts([Inter, Roboto], async () => {});
  });
});
