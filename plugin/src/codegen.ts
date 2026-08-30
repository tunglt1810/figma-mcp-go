// Dev Mode codegen.
//
// The manifest declared `capabilities: ["inspect"]` without ever registering a
// codegen provider, so Dev Mode's Code panel showed Figma's generic output and
// nothing this project knows. Registering the provider needs `"codegen"` in
// capabilities as well — without it Figma never runs the plugin in codegen
// mode, and rejects `codegenPreferences` in the manifest while it is at it.
//
// Live generation on demand would mean the panel asking the MCP client for a
// completion mid-render — a second request direction through the bridge, plus
// MCP sampling, which is optional in the protocol and absent from the clients
// this server targets. So the flow is inverted: the client generates code with
// the whole repository in front of it and stores it on the node, and the panel
// serves what is stored. The code is written into shared plugin data, so it
// travels with the file and every teammate's Dev Mode sees it, not just the
// machine that generated it.

export const PLUGIN_DATA_NAMESPACE = "figma-mcp-go";
export const CODEGEN_KEY = "codegen";

export interface CodegenBlock {
  title: string;
  language: string;
  code: string;
}

/** Figma rejects a language it does not know, so unknown ones fall back. */
const KNOWN_LANGUAGES = new Set([
  "TYPESCRIPT", "JAVASCRIPT", "HTML", "CSS", "JSON", "GRAPHQL", "PYTHON",
  "GO", "SQL", "SWIFT", "KOTLIN", "RUBY", "CPP", "RUST", "BASH", "PLAINTEXT",
]);

export function normalizeLanguage(language: string | undefined): string {
  const upper = String(language ?? "").toUpperCase();
  return KNOWN_LANGUAGES.has(upper) ? upper : "PLAINTEXT";
}

/** Parse stored blocks, tolerating anything that is not what we wrote. */
export function parseBlocks(raw: string | undefined): CodegenBlock[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter((block: any) => block && typeof block.code === "string")
      .map((block: any) => ({
        title: String(block.title ?? "Code"),
        language: normalizeLanguage(block.language),
        code: block.code,
      }));
  } catch {
    // Written by an older version, or by hand. Showing nothing beats throwing
    // inside Figma's render path, which surfaces as a broken panel.
    return [];
  }
}

export function readBlocks(node: any): CodegenBlock[] {
  if (!node || typeof node.getSharedPluginData !== "function") return [];
  return parseBlocks(node.getSharedPluginData(PLUGIN_DATA_NAMESPACE, CODEGEN_KEY));
}

/**
 * Find the code to show for a selected node.
 *
 * A designer clicks the instance, not the component that defines it, and often
 * clicks a layer inside it. So: the node itself, then what it is an instance
 * of, then up through its ancestors — the first that carries code wins.
 */
export async function resolveBlocks(node: any): Promise<{ blocks: CodegenBlock[]; source: any } | null> {
  let current = node;
  while (current) {
    const own = readBlocks(current);
    if (own.length > 0) return { blocks: own, source: current };

    if (current.type === "INSTANCE" && typeof current.getMainComponentAsync === "function") {
      const main = await current.getMainComponentAsync();
      if (main) {
        const fromMain = readBlocks(main);
        if (fromMain.length > 0) return { blocks: fromMain, source: main };
        // A variant's code usually lives on the set, not the single variant.
        if (main.parent?.type === "COMPONENT_SET") {
          const fromSet = readBlocks(main.parent);
          if (fromSet.length > 0) return { blocks: fromSet, source: main.parent };
        }
      }
    }

    current = current.parent;
    if (current && (current.type === "PAGE" || current.type === "DOCUMENT")) break;
  }
  return null;
}

/** The manifest's default for the language preference: show everything. */
export const ALL_LANGUAGES = "ALL";

/**
 * Narrow the blocks to the language the viewer picked in the Code panel.
 *
 * A node often carries several blocks — the component in TypeScript, its CSS,
 * the GraphQL query behind it — and a developer working in one of them does not
 * want the other two. A language with nothing stored for this node falls back
 * to everything rather than to an empty panel: the viewer's standing preference
 * should not read as "this node has no code".
 */
export function selectBlocks(blocks: CodegenBlock[], language: string | undefined): CodegenBlock[] {
  const wanted = String(language ?? ALL_LANGUAGES).toUpperCase();
  if (wanted === ALL_LANGUAGES) return blocks;
  const matching = blocks.filter((block) => block.language === wanted);
  return matching.length > 0 ? matching : blocks;
}

/** Register the Dev Mode provider. Safe to call outside codegen mode. */
export function registerCodegen(): boolean {
  const api: any = typeof figma !== "undefined" ? figma : null;
  if (!api?.codegen || typeof api.codegen.on !== "function") return false;

  api.codegen.on("generate", async (event: any) => {
    const found = await resolveBlocks(event?.node);
    if (!found) {
      return [
        {
          title: "Figma MCP Go",
          language: "PLAINTEXT",
          code:
            "No code stored for this node yet.\n\n" +
            "Ask your AI tool to generate the code for it and call\n" +
            "set_codegen_result — what it writes shows up here for everyone.",
        },
      ];
    }
    // The event carries the preferences as of this render; api.codegen.preferences
    // is the same value and is read only as a fallback for older editors.
    const language = event?.language ?? api.codegen.preferences?.language;
    return selectBlocks(found.blocks, language).map((block) => ({
      title: block.title,
      language: block.language,
      code: block.code,
    }));
  });

  // Figma does not re-render the Code panel on its own when a preference
  // changes — the handler above only runs again if something asks for it.
  api.codegen.on("preferenceschange", async () => {
    if (typeof api.codegen.refresh === "function") api.codegen.refresh();
  });
  return true;
}
