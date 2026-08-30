import { describe, expect, it } from "bun:test";
import {
  ALL_LANGUAGES,
  CODEGEN_KEY,
  PLUGIN_DATA_NAMESPACE,
  normalizeLanguage,
  parseBlocks,
  readBlocks,
  registerCodegen,
  resolveBlocks,
  selectBlocks,
} from "./codegen";

const withCode = (node: any, blocks: any) => ({
  ...node,
  getSharedPluginData: (ns: string, key: string) =>
    ns === PLUGIN_DATA_NAMESPACE && key === CODEGEN_KEY ? JSON.stringify(blocks) : "",
});

const bare = (node: any) => ({ ...node, getSharedPluginData: () => "" });

describe("normalizeLanguage", () => {
  it("uppercases a language Figma knows", () => {
    expect(normalizeLanguage("typescript")).toBe("TYPESCRIPT");
  });

  // Figma rejects a language it does not know, which would break the panel.
  it("falls back for anything else", () => {
    expect(normalizeLanguage("brainfuck")).toBe("PLAINTEXT");
    expect(normalizeLanguage(undefined)).toBe("PLAINTEXT");
  });
});

describe("parseBlocks", () => {
  it("reads what was written", () => {
    expect(parseBlocks(JSON.stringify([{ title: "Button", language: "tsx", code: "x" }])))
      .toEqual([{ title: "Button", language: "PLAINTEXT", code: "x" }]);
  });

  it("defaults a missing title", () => {
    expect(parseBlocks(JSON.stringify([{ language: "css", code: "a{}" }]))[0].title).toBe("Code");
  });

  it("drops entries with no code", () => {
    expect(parseBlocks(JSON.stringify([{ title: "x" }, { code: "y" }])).length).toBe(1);
  });

  // Throwing inside Figma's render path surfaces as a broken panel.
  it("returns nothing for junk rather than throwing", () => {
    expect(parseBlocks("not json")).toEqual([]);
    expect(parseBlocks(JSON.stringify({ nope: true }))).toEqual([]);
    expect(parseBlocks("")).toEqual([]);
    expect(parseBlocks(undefined)).toEqual([]);
  });
});

describe("readBlocks", () => {
  it("returns nothing for a node with no plugin data API", () => {
    expect(readBlocks({})).toEqual([]);
    expect(readBlocks(null)).toEqual([]);
  });
});

describe("resolveBlocks", () => {
  const blocks = [{ title: "Button", language: "TYPESCRIPT", code: "export const Button = () => null" }];

  it("uses the node's own code", async () => {
    const node = withCode({ id: "1:1", type: "FRAME" }, blocks);
    const found = await resolveBlocks(node);
    expect(found?.source).toBe(node);
    expect(found?.blocks[0].code).toContain("Button");
  });

  // A designer clicks the instance, not the component that defines it.
  it("falls back to the instance's main component", async () => {
    const main = withCode({ id: "2:1", type: "COMPONENT" }, blocks);
    const instance = {
      ...bare({ id: "1:1", type: "INSTANCE" }),
      getMainComponentAsync: async () => main,
    };
    const found = await resolveBlocks(instance);
    expect(found?.source).toBe(main);
  });

  it("falls back to the component set when a variant has none", async () => {
    const set = withCode({ id: "3:0", type: "COMPONENT_SET" }, blocks);
    const main = { ...bare({ id: "2:1", type: "COMPONENT" }), parent: set };
    const instance = {
      ...bare({ id: "1:1", type: "INSTANCE" }),
      getMainComponentAsync: async () => main,
    };
    expect((await resolveBlocks(instance))?.source).toBe(set);
  });

  // Clicking a layer inside a card should still find the card's code.
  it("walks up to an ancestor", async () => {
    const card = withCode({ id: "1:1", type: "FRAME" }, blocks);
    const label = { ...bare({ id: "1:2", type: "TEXT" }), parent: card };
    expect((await resolveBlocks(label))?.source).toBe(card);
  });

  it("stops at the page rather than reading the whole document", async () => {
    const page = withCode({ id: "1:0", type: "PAGE" }, blocks);
    const frame = { ...bare({ id: "1:1", type: "FRAME" }), parent: page };
    expect(await resolveBlocks(frame)).toBeNull();
  });

  it("returns nothing when nothing up the chain has code", async () => {
    const frame = bare({ id: "1:1", type: "FRAME" });
    expect(await resolveBlocks(frame)).toBeNull();
  });

  it("survives an instance whose main component is gone", async () => {
    const instance = {
      ...bare({ id: "1:1", type: "INSTANCE" }),
      getMainComponentAsync: async () => null,
    };
    expect(await resolveBlocks(instance)).toBeNull();
  });
});

describe("selectBlocks", () => {
  const blocks = [
    { title: "Component", language: "TYPESCRIPT", code: "a" },
    { title: "Styles", language: "CSS", code: "b" },
  ];

  it("keeps everything on the manifest's default", () => {
    expect(selectBlocks(blocks, "ALL")).toEqual(blocks);
    expect(selectBlocks(blocks, undefined)).toEqual(blocks);
  });

  it("narrows to the language the viewer picked", () => {
    expect(selectBlocks(blocks, "CSS").map(b => b.title)).toEqual(["Styles"]);
  });

  // A standing preference must not read as "this node has no code".
  it("falls back to everything when the node has nothing in that language", () => {
    expect(selectBlocks(blocks, "PYTHON")).toEqual(blocks);
  });
});

describe("registerCodegen", () => {
  const fakeCodegen = () => {
    const handlers: Record<string, Function> = {};
    let refreshed = 0;
    (globalThis as any).figma = {
      codegen: {
        on: (event: string, handler: Function) => { handlers[event] = handler; },
      },
    };
    return { handlers };
  };

  it("applies the language the generate event carries", async () => {
    const { handlers } = fakeCodegen();
    expect(registerCodegen()).toBe(true);
    const node = withCode({ type: "FRAME", parent: null }, [
      { title: "Component", language: "TYPESCRIPT", code: "a" },
      { title: "Styles", language: "CSS", code: "b" },
    ]);
    const result = await handlers["generate"]({ node, language: "CSS" });
    expect(result.map((b: any) => b.title)).toEqual(["Styles"]);
  });
});

// Figma validates the manifest itself and only reports the failure inside the
// desktop app, where no test run would see it. These are the two rules it
// applied to this plugin: a codegen provider needs the capability, and a
// codegen plugin must offer at least one language.
describe("manifest", () => {
  const manifest = require("../manifest.json");

  it("declares the codegen capability the provider needs", () => {
    expect(manifest.capabilities).toContain("codegen");
  });

  it("offers at least one language, whose values selectBlocks understands", () => {
    expect(manifest.codegenLanguages.length).toBeGreaterThan(0);
    for (const { label, value } of manifest.codegenLanguages) {
      expect(label).toBeTruthy();
      // Anything else would filter every block out and fall back to all of
      // them, making the dropdown entry a no-op.
      if (value !== ALL_LANGUAGES) expect(normalizeLanguage(value)).toBe(value);
    }
  });

  // Figma takes the first entry as the default, and "show everything" is the
  // only honest default for a panel that serves whatever was stored.
  it("defaults to all languages", () => {
    expect(manifest.codegenLanguages[0].value).toBe(ALL_LANGUAGES);
  });
});
