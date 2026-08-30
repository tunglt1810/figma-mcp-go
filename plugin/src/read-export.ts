import { HandlerMap } from "./dispatch";
import { reportProgress, stepProgress } from "./progress";
export const readExportHandlers: HandlerMap = {
  "get_screenshot": async (request) => {
    const format =
      request.params && request.params.format
        ? request.params.format
        : "PNG";
    const scale =
      request.params && request.params.scale != null
        ? request.params.scale
        : 2;
    let targetNodes: any[];
    if (request.nodeIds && request.nodeIds.length > 0) {
      const nodes = await Promise.all(
        request.nodeIds.map((id: string) => figma.getNodeByIdAsync(id)),
      );
      targetNodes = nodes.filter(
        (n) => n !== null && n.type !== "DOCUMENT" && n.type !== "PAGE",
      );
    } else {
      targetNodes = figma.currentPage.selection.slice();
    }
    if (targetNodes.length === 0)
      throw new Error(
        "No nodes to export. Select nodes or provide nodeIds.",
      );
    // Sequential rather than Promise.all: exporting is the slowest read there
    // is, and a caller that asked for twenty frames wants to know how far in it
    // has got. Figma renders them one at a time regardless.
    const exports: any[] = [];
    for (let i = 0; i < targetNodes.length; i++) {
      const node: any = targetNodes[i];
      if (targetNodes.length > 1) {
        await reportProgress(
          request.requestId,
          stepProgress(i, targetNodes.length),
          `Exporting ${node.name} (${i + 1}/${targetNodes.length})`,
        );
      }
      const settings: any =
        format === "SVG"
          ? { format: "SVG" }
          : format === "PDF"
            ? { format: "PDF" }
            : { format, constraint: { type: "SCALE", value: scale } };
      const bytes = await node.exportAsync(settings);
      const base64 = figma.base64Encode(bytes);
      exports.push({
        nodeId: node.id,
        nodeName: node.name,
        format,
        base64,
        width: node.width,
        height: node.height,
      });
    }
    return {
      type: request.type,
      requestId: request.requestId,
      data: { exports },
    };
  },

  "export_frames_to_pdf": async (request) => {
    const nodeIds: string[] = request.nodeIds ?? [];
    if (nodeIds.length === 0) {
      throw new Error("nodeIds is required and must not be empty");
    }
    const frames: any[] = [];
    for (let i = 0; i < nodeIds.length; i++) {
      const id = nodeIds[i];
      if (nodeIds.length > 1) {
        await reportProgress(
          request.requestId,
          stepProgress(i, nodeIds.length),
          `Rendering page ${i + 1}/${nodeIds.length}`,
        );
      }
      const node = await figma.getNodeByIdAsync(id);
      if (!node || node.type === "DOCUMENT" || node.type === "PAGE") {
        throw new Error(`Node ${id} not found or is not exportable`);
      }
      const bytes = await (node as any).exportAsync({ format: "PDF" });
      const base64 = figma.base64Encode(bytes);
      frames.push({
        nodeId: node.id,
        nodeName: node.name,
        base64,
      });
    }
    return {
      type: request.type,
      requestId: request.requestId,
      data: { frames },
    };
  },

  "get_image_bytes": async (request) => {
    const nodeIds: string[] = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");

    // The original bytes, not a re-render. get_screenshot rasterises what the
    // node looks like now; this returns the asset that was placed, which is
    // what a build needs to ship.
    const images: any[] = [];
    const skipped: { nodeId: string; reason: string }[] = [];
    const seen = new Set<string>();

    for (const nodeId of nodeIds) {
      const node = await figma.getNodeByIdAsync(nodeId) as any;
      if (!node) { skipped.push({ nodeId, reason: "Node not found" }); continue; }
      const fills = node.fills;
      if (!Array.isArray(fills)) {
        skipped.push({ nodeId, reason: `${node.type} has no fills to read` });
        continue;
      }
      const imageFills = fills.filter((fill: any) => fill.type === "IMAGE" && fill.imageHash);
      if (imageFills.length === 0) {
        skipped.push({ nodeId, reason: "No image fill on this node" });
        continue;
      }
      for (const fill of imageFills) {
        // One picture used in ten places is one asset. Sending it ten times
        // would be the bulk of the response.
        if (seen.has(fill.imageHash)) continue;
        seen.add(fill.imageHash);
        const image = figma.getImageByHash(fill.imageHash);
        if (!image) {
          skipped.push({ nodeId, reason: `Image ${fill.imageHash} is no longer in the file` });
          continue;
        }
        const bytes = await image.getBytesAsync();
        images.push({
          nodeId,
          nodeName: node.name,
          imageHash: fill.imageHash,
          scaleMode: fill.scaleMode,
          base64: figma.base64Encode(bytes),
          bytes: bytes.length,
        });
      }
    }

    return {
      type: request.type,
      requestId: request.requestId,
      data: { images, skipped },
    };
  },
};

export const handleReadExportRequest = async (request: any): Promise<any> => {
  const handler = readExportHandlers[request.type];
  return handler ? handler(request) : null;
};
