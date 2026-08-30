// Loading fonts, and saying which ones are missing.
//
// figma.loadFontAsync rejects for a font the file does not have. Called one at a
// time in the middle of a run, the first rejection aborts everything after it —
// so a text edit could land half-applied, and the caller learned about exactly
// one missing font per attempt even when three were missing.
//
// Loading them together instead means one error that names every missing font,
// raised before any text is touched.

export interface FontName {
  family: string;
  style: string;
}

/** How Figma names a font in an error, and how fonts are deduplicated here. */
export const fontKey = (font: FontName): string => `${font.family} ${font.style}`;

/**
 * Load every font, then report all the failures at once.
 *
 * The loads run together and are all allowed to settle: aborting on the first
 * rejection is what hid the other missing fonts. Fonts already loaded resolve
 * immediately, so calling this with a node's whole font list is cheap.
 */
export async function loadFonts(
  fonts: FontName[],
  load: (font: FontName) => Promise<void> = (font) => figma.loadFontAsync(font),
): Promise<void> {
  const unique = new Map<string, FontName>();
  for (const font of fonts) {
    if (font && font.family && font.style) unique.set(fontKey(font), font);
  }
  if (unique.size === 0) return;

  const outcomes = await Promise.allSettled(
    [...unique.values()].map((font) => load(font)),
  );
  const missing = [...unique.keys()].filter((_, i) => outcomes[i].status === "rejected");
  if (missing.length === 0) return;

  throw new Error(
    missing.length === 1
      ? `Font not available in this file: ${missing[0]}. Add it in Figma, or pick a font the file already has — get_fonts lists them.`
      : `Fonts not available in this file: ${missing.join(", ")}. Add them in Figma, or pick fonts the file already has — get_fonts lists them.`,
  );
}
