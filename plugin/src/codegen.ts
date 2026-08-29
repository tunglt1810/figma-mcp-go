// Dev Mode codegen.
//
// The manifest has always declared `capabilities: ["inspect"]` without ever
// registering a codegen provider, so Dev Mode's Code panel showed Figma's
// generic output and nothing this project knows.
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
    return found.blocks.map((block) => ({
      title: block.title,
      language: block.language,
      code: block.code,
    }));
  });
  return true;
}
