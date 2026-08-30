import { describe, it, expect } from "bun:test";
import { copyTextToClipboard } from "./copy-helper";

describe("copyTextToClipboard", () => {
  it("should send WebSocket message when WebSocket is open", async () => {
    let sentMessage = "";
    const mockSocket = {
      readyState: 1,
      OPEN: 1,
      send: (msg: string) => {
        sentMessage = msg;
      },
    };

    const result = await copyTextToClipboard("node-123", { socket: mockSocket });
    expect(result.success).toBe(true);
    expect(result.viaWS).toBe(true);
    expect(JSON.parse(sentMessage)).toEqual({
      type: "copy_to_clipboard",
      text: "node-123",
    });
  });

  // execCommand("copy") copies the document's selection, not `text`, so running
  // it after the server already has the id can only overwrite it.
  it("does not touch the browser clipboard once the server has the text", async () => {
    let execCalled = false;
    const result = await copyTextToClipboard("node-123", {
      socket: { readyState: 1, OPEN: 1, send: () => {} },
      execCommand: () => { execCalled = true; return true; },
      writeText: async () => { throw new Error("should not be reached"); },
    });

    expect(result.viaWS).toBe(true);
    expect(execCalled).toBe(false);
  });

  it("should fallback to execCommand when WebSocket is disconnected", async () => {
    let execCalled = false;
    const mockExec = (cmd: string) => {
      if (cmd === "copy") {
        execCalled = true;
        return true;
      }
      return false;
    };

    const result = await copyTextToClipboard("node-456", {
      socket: null,
      execCommand: mockExec,
    });

    expect(result.success).toBe(true);
    expect(result.viaWS).toBe(false);
    expect(execCalled).toBe(true);
  });

  it("should fallback to writeText if execCommand fails", async () => {
    let textWritten = "";
    const mockExec = () => false;
    const mockWriteText = async (text: string) => {
      textWritten = text;
    };

    const result = await copyTextToClipboard("node-789", {
      socket: null,
      execCommand: mockExec,
      writeText: mockWriteText,
    });

    expect(result.success).toBe(true);
    expect(result.viaWS).toBe(false);
    expect(textWritten).toBe("node-789");
  });

  it("should report failure when all copy methods fail", async () => {
    const mockExec = () => false;
    const mockWriteText = async () => {
      throw new Error("NotAllowedError: browser blocked unattended copy");
    };

    const result = await copyTextToClipboard("node-000", {
      socket: null,
      execCommand: mockExec,
      writeText: mockWriteText,
    });

    expect(result.success).toBe(false);
    expect(result.viaWS).toBe(false);
  });
});
