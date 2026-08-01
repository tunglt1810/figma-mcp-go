import { handleWriteCreateRequest } from "./write-create";
import { handleWriteModifyRequest } from "./write-modify";
import { handleWriteStyleRequest } from "./write-styles";
import { handleWriteVariableRequest } from "./write-variables";
import { handleWriteComponentRequest } from "./write-components";
import { handleWritePrototypeRequest } from "./write-prototype";
import { handleWritePageRequest } from "./write-page";
import { handleBatchPipelineRequest } from "./batch-pipeline";

export const handleWriteRequest = async (request: any): Promise<any> => {
  const dispatchSingle = async (subReq: any) =>
    (await handleWriteCreateRequest(subReq)) ??
    (await handleWriteModifyRequest(subReq)) ??
    (await handleWriteStyleRequest(subReq)) ??
    (await handleWriteVariableRequest(subReq)) ??
    (await handleWriteComponentRequest(subReq)) ??
    (await handleWritePrototypeRequest(subReq)) ??
    (await handleWritePageRequest(subReq));

  return (
    (await handleBatchPipelineRequest(request, dispatchSingle)) ??
    (await dispatchSingle(request))
  );
};


