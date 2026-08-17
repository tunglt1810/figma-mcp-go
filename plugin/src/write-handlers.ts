import { HandlerMap, mergeHandlers } from "./dispatch";
import { writeCreateHandlers } from "./write-create";
import { writeModifyHandlers } from "./write-modify";
import { writeStylesHandlers } from "./write-styles";
import { writeVariablesHandlers } from "./write-variables";
import { writeComponentsHandlers } from "./write-components";
import { writePrototypeHandlers } from "./write-prototype";
import { writePageHandlers } from "./write-page";
import { handleBatchPipelineRequest } from "./batch-pipeline";

export const writeHandlers: HandlerMap = mergeHandlers(
  writeCreateHandlers,
  writeModifyHandlers,
  writeStylesHandlers,
  writeVariablesHandlers,
  writeComponentsHandlers,
  writePrototypeHandlers,
  writePageHandlers,
);

export const handleWriteRequest = async (request: any): Promise<any> => {
  const dispatchSingle = async (subReq: any) => {
    const handler = writeHandlers[subReq.type];
    return handler ? handler(subReq) : null;
  };

  // The pipeline takes the whole request before dispatch, because it runs the
  // steps itself and needs a dispatcher to run them with.
  return (
    (await handleBatchPipelineRequest(request, dispatchSingle)) ??
    (await dispatchSingle(request))
  );
};
