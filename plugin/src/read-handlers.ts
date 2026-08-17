import { HandlerMap, mergeHandlers } from "./dispatch";
import { readDocumentHandlers } from "./read-document";
import { readStylesHandlers } from "./read-styles";
import { readExportHandlers } from "./read-export";

export const readHandlers: HandlerMap = mergeHandlers(
  readDocumentHandlers,
  readStylesHandlers,
  readExportHandlers,
);

export const handleReadRequest = async (request: any): Promise<any> => {
  const handler = readHandlers[request.type];
  return handler ? handler(request) : null;
};
