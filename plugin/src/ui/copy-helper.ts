export interface CopyOptions {
  socket?: { readyState: number; OPEN: number; send: (msg: string) => void } | null;
  execCommand?: (command: string) => boolean;
  writeText?: (text: string) => Promise<void>;
}

export async function copyTextToClipboard(
  text: string,
  options: CopyOptions
): Promise<{ success: boolean; viaWS: boolean }> {
  let success = false;
  let viaWS = false;

  // 1. Send via WebSocket if connected to the Go server (avoids the browser iframe user-gesture rule)
  if (options.socket && options.socket.readyState === options.socket.OPEN) {
    options.socket.send(JSON.stringify({ type: "copy_to_clipboard", text }));
    success = true;
    viaWS = true;
  }

  // 2. Try DOM execCommand if available. Skip it once the server has the text:
  // it copies the document's selection, not `text`, so running it after a
  // successful send could only replace what the server just put on the clipboard.
  if (!success && options.execCommand) {
    try {
      if (options.execCommand("copy")) {
        success = true;
      }
    } catch {
      // execCommand failed or is restricted
    }
  }

  // 3. Fall back to the async writeText API if execCommand failed
  if (!success && options.writeText) {
    try {
      await options.writeText(text);
      success = true;
    } catch {
      // writeText failed or is restricted
    }
  }

  return { success, viaWS };
}
