import { HandlerMap } from "./dispatch";
import { makeSolidPaint } from "./write-helpers";
import { FontName, loadFonts } from "./fonts";

// Rich text.
//
// set_text writes the whole node in one font and one colour, which is all a
// heading needs and nothing a paragraph does: a sentence with one bold word, a
// link, or a bulleted list could not be expressed at all. Figma models these as
// ranges over the characters, so the tool does too.

export interface TextRange {
  start: number;
  end: number;
  [key: string]: any;
}

/**
 * Check a range against the node's text and return it as a pair.
 *
 * Figma throws a bare "Error: in setRangeFontName" for an out-of-bounds range,
 * naming neither the range nor the length, so the check happens here where both
 * are known.
 */
export function resolveRange(range: TextRange, length: number): [number, number] {
  const start = Number(range.start);
  const end = Number(range.end);
  if (!Number.isInteger(start) || !Number.isInteger(end)) {
    throw new Error(
      `Range {start: ${range.start}, end: ${range.end}} must use whole numbers`,
    );
  }
  if (start < 0 || end > length) {
    throw new Error(
      `Range ${start}-${end} is outside the text, which is ${length} character(s) long`,
    );
  }
  if (start >= end) {
    throw new Error(`Range ${start}-${end} is empty — end must be greater than start`);
  }
  return [start, end];
}

/**
 * The font to apply to a range.
 *
 * Figma takes a complete {family, style} pair, but a caller usually means "make
 * this bold" and gives only the style. The missing half comes from the text
 * already in the range; when that range is itself mixed, Figma reports a symbol
 * and there is nothing to inherit, so the caller has to say.
 */
export function resolveRangeFont(
  range: TextRange,
  current: any,
): { family: string; style: string } | null {
  if (!range.fontFamily && !range.fontStyle) return null;
  const inherited = typeof current === "symbol" || !current ? null : current;
  const family = range.fontFamily ?? inherited?.family;
  const style = range.fontStyle ?? inherited?.style;
  if (!family || !style) {
    throw new Error(
      "fontFamily and fontStyle are both required for a range whose existing font is mixed",
    );
  }
  return { family, style };
}

/** Ranges sorted so overlapping edits apply left to right, as written. */
export function sortRanges(ranges: TextRange[]): TextRange[] {
  return [...ranges].sort((a, b) => Number(a.start) - Number(b.start));
}

const applyRange = async (node: any, range: TextRange, font: FontName | null) => {
  const [start, end] = resolveRange(range, node.characters.length);

  // The font was resolved and loaded before the first range was written, and is
  // applied exactly as resolved. Resolving it again here would read a node an
  // earlier range has already changed, so the font written would be one that was
  // never loaded — and Figma refuses it, half way through the edit.
  if (font) node.setRangeFontName(start, end, font);

  if (range.fontSize != null) node.setRangeFontSize(start, end, Number(range.fontSize));
  if (range.color != null) {
    node.setRangeFills(start, end, [makeSolidPaint(range.color, range.opacity)]);
  }
  if (range.textDecoration) node.setRangeTextDecoration(start, end, range.textDecoration);
  if (range.textCase) node.setRangeTextCase(start, end, range.textCase);
  if (range.letterSpacing != null) {
    node.setRangeLetterSpacing(start, end, {
      value: Number(range.letterSpacing),
      unit: range.letterSpacingUnit || "PIXELS",
    });
  }
  if (range.lineHeight != null) {
    node.setRangeLineHeight(start, end,
      range.lineHeight === "AUTO"
        ? { unit: "AUTO" }
        : { value: Number(range.lineHeight), unit: range.lineHeightUnit || "PIXELS" },
    );
  }
  if (range.listType) node.setRangeListOptions(start, end, { type: range.listType });
  if (range.indentation != null) {
    node.setRangeIndentation(start, end, Number(range.indentation));
  }
  // null clears a link; a string sets one. Absent leaves it alone.
  if (range.hyperlink !== undefined) {
    node.setRangeHyperlink(
      start,
      end,
      range.hyperlink === null ? null : { type: "URL", value: range.hyperlink },
    );
  }
};

/**
 * Load every font the node already uses.
 *
 * Any range edit rewrites part of a node whose other parts keep their fonts,
 * and Figma refuses to touch a text node while any font in it is unloaded.
 */
export const loadNodeFonts = async (node: any) => {
  await loadFonts(node.getRangeAllFontNames(0, node.characters.length));
};

/**
 * The font each range asks for, resolved against the node as the caller found
 * it — one entry per range, in the same order, null where the range asks for no
 * font change.
 *
 * Resolved before anything is written, for two reasons. A range asking for a
 * font the file does not have used to fail on that range, after the ranges
 * before it had already been applied — so the node was left in a state nobody
 * asked for and the caller learned about one missing font at a time. And a range
 * that inherits half its font from the text would otherwise resolve differently
 * at load time and at write time, once an earlier range had changed that text,
 * so the font written was one that was never loaded.
 */
export const rangeFonts = (node: any, ranges: TextRange[]): (FontName | null)[] =>
  ranges.map(range => {
    const [start, end] = resolveRange(range, node.characters.length);
    return resolveRangeFont(range, node.getRangeFontName(start, end));
  });

const getTextNode = async (nodeId: string | undefined) => {
  if (!nodeId) throw new Error("nodeId is required");
  const node = await figma.getNodeByIdAsync(nodeId);
  if (!node) throw new Error(`Node not found: ${nodeId}`);
  if (node.type !== "TEXT") throw new Error(`Node ${nodeId} is not a TEXT node`);
  return node as any;
};

export const writeTextHandlers: HandlerMap = {
  "set_text_ranges": async (request) => {
    const p = request.params || {};
    const node = await getTextNode(request.nodeIds && request.nodeIds[0]);
    const ranges: TextRange[] = Array.isArray(p.ranges) ? p.ranges : [];
    if (ranges.length === 0) throw new Error("ranges is required and must not be empty");
    if (node.characters.length === 0) {
      throw new Error(`Node ${node.id} has no text — use set_text first`);
    }

    const sorted = sortRanges(ranges);
    // One load for the node's existing fonts and every font the ranges ask for.
    // Nothing is written until they are all in.
    const fonts = rangeFonts(node, sorted);
    await loadFonts([
      ...node.getRangeAllFontNames(0, node.characters.length),
      ...fonts.filter((font): font is FontName => font !== null),
    ]);
    for (let i = 0; i < sorted.length; i++) {
      await applyRange(node, sorted[i], fonts[i]);
    }

    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        id: node.id,
        name: node.name,
        characters: node.characters,
        rangesApplied: ranges.length,
      },
    };
  },
};
