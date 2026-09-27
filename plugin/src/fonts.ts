// Loading fonts, and reporting which ones are missing.
//
// figma.loadFontAsync rejects a font the file does not have. When fonts were
// loaded one at a time during a run, the first rejection stopped everything
// after it. So a text edit could be half-applied, and the caller learned
// about only one missing font per attempt, even when three were missing.
//
// Loading them together gives one error that names every missing font, before
// any text is touched.

export interface FontName {
  family: string;
  style: string;
}

/** How Figma names a font in an error, and how fonts are deduplicated here. */
export const fontKey = (font: FontName): string => `${font.family} ${font.style}`;

/**
 * Load every font, then report all the failures at once.
 *
 * The loads run together, and all of them are allowed to finish. Stopping at
 * the first rejection is what hid the other missing fonts. Fonts that are
 * already loaded resolve at once, so calling this with a node's full font
 * list is cheap.
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
