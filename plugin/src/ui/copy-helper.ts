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

  // 1. Send via WebSocket if connected to Go Server (bypasses browser iframe user gesture policy)
  if (options.socket && options.socket.readyState === options.socket.OPEN) {
    options.socket.send(JSON.stringify({ type: "copy_to_clipboard", text }));
    success = true;
    viaWS = true;
  }

  // 2. Try DOM execCommand if available. Skipped once the server has it: this
  // copies the document's selection rather than `text`, so running it after a
  // successful send can only replace what the server just put on the clipboard.
  if (!success && options.execCommand) {
    try {
      if (options.execCommand("copy")) {
        success = true;
      }
    } catch {
      // execCommand failed or restricted
    }
  }

  // 3. Fallback to async writeText API if execCommand failed
  if (!success && options.writeText) {
    try {
      await options.writeText(text);
      success = true;
    } catch {
      // writeText failed or restricted
    }
  }

  return { success, viaWS };
}
