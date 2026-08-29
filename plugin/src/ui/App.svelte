<script lang="ts">
  import { onMount } from "svelte";
  import { copyTextToClipboard } from "./copy-helper";
  import { versionWarning, versionWarningSummary } from "./version-check";
  import { DEFAULT_HOST, DEFAULT_PORT, GUARD_MODES, normalizeStoredPrefs, sanitizeHost, sanitizePort } from "./prefs";
  import type { GuardMode } from "./prefs";
  import { finishEntry, formatDuration, formatLog, progressEntry, startEntry } from "./activity";
  import type { ActivityEntry } from "./activity";
  import { destructiveReason, isDestructive, isMutating } from "../tool-classes";

  let connected = false;
  let fileName = "—";
  let pageName = "—";
  let selectionCount = 0;
  let selectedNodes: { id: string, name: string }[] = [];
  // Running work is read off the activity log rather than tracked separately —
  // one source of truth means the banner and the log can never disagree.
  let activityLog: ActivityEntry[] = [];
  $: runningEntries = activityLog.filter(entry => entry.status === "running");
  $: isWorking = runningEntries.length > 0;
  $: currentTool = runningEntries.length > 0 ? runningEntries[0].tool : "";

  // `now` ticks only while something is running, so a finished panel is not
  // re-rendering once a second for nothing.
  let now = Date.now();
  let tick: ReturnType<typeof setInterval> | null = null;
  $: {
    if (isWorking && tick === null) {
      tick = setInterval(() => { now = Date.now(); }, 200);
    } else if (!isWorking && tick !== null) {
      clearInterval(tick);
      tick = null;
      now = Date.now();
    }
  }

  let guardMode: GuardMode = "off";
  let showLog = false;
  // Requests held for the user to approve, oldest first. The server is still
  // waiting on each one, so nothing may be dropped silently.
  let pendingApprovals: { payload: any; reason: string }[] = [];
  $: pendingApproval = pendingApprovals.length > 0 ? pendingApprovals[0] : null;

  const PANEL_WIDTH = 320;
  const PANEL_HEIGHT = 230;
  const PANEL_HEIGHT_WITH_LOG = 460;
  
  // Defaults to on; the stored value replaces it once the core answers.
  let autoCopyEnabled = true;
  let copyError = false;
  let autoCopyBroken = false; // sticky: true once an unattended auto-copy attempt has failed

  // Configurable server address.
  // Persisted via figma.clientStorage (through plugin core) because localStorage
  // is unavailable inside Figma's data: URL sandbox.
  let serverHost = DEFAULT_HOST;
  let serverPort = DEFAULT_PORT;
  let serverVersion = "";

  const pluginVersion = __APP_VERSION__;
  // Sent by the plugin core once it starts. It can arrive either side of the
  // socket opening, so both paths announce, and neither assumes the other ran.
  let pluginHandlers: string[] = [];
  // Recomputed whenever the server reports its version — null while
  // disconnected, or when the two versions are close enough not to matter.
  $: versionMismatch = connected ? versionWarningSummary(pluginVersion, serverVersion) : null;
  $: versionMismatchDetail = connected ? versionWarning(pluginVersion, serverVersion) : null;

  let showSettings = false;
  let editHost = serverHost;
  let editPort = serverPort;

  const RECONNECT_DELAY_MS = 1500;

  let socket: WebSocket | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let configLoaded = false;

  function connect() {
    // Detach the old handler before closing so its onclose doesn't fire
    // after we've already assigned a new socket, which would null out the
    // new reference and silently break the connection.
    if (socket) {
      socket.onclose = null;
      socket.close();
    }
    const ws = new WebSocket(`ws://${serverHost}:${serverPort}/ws`);
    socket = ws;

    ws.onopen = () => {
      connected = true;
      ws.send(JSON.stringify({ type: "get_server_info" }));
      // Tell the server what it is talking to, so a mismatch is visible in its
      // log too — the user reporting a bug may never open this panel.
      announce(ws);
      parent.postMessage({ pluginMessage: { type: "ui-ready" } }, "*");
    };

    ws.onclose = () => {
      if (socket !== ws) return; // stale handler — a newer connect() already took over
      connected = false;
      serverVersion = "";
      socket = null;
      // Anything still running will never get an answer through this socket.
      // Closing the entries out beats leaving a spinner that never stops.
      activityLog = activityLog.map(entry =>
        entry.status === "running"
          ? { ...entry, status: "error" as const, endedAt: Date.now(), message: "connection lost" }
          : entry,
      );
      if (reconnectTimer === null) {
        reconnectTimer = setTimeout(() => {
          reconnectTimer = null;
          connect();
        }, RECONNECT_DELAY_MS);
      }
    };

    ws.onerror = () => {
      connected = false;
    };

    ws.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data);
        if (payload.type === "server-info") {
          serverVersion = payload.version ?? "";
          return;
        }
        if (payload.type === "cancel_request") {
          cancelRequest(payload.requestId);
          return;
        }
        if (payload.requestId) {
          admitRequest(payload);
          return;
        }
        parent.postMessage({ pluginMessage: { type: "server-request", payload } }, "*");
      } catch {
        // ignore malformed frames
      }
    };
  }

  /** Tell the server what it is talking to. */
  function announce(ws: WebSocket) {
    ws.send(JSON.stringify({
      type: "plugin-info",
      version: pluginVersion,
      handlers: pluginHandlers,
    }));
  }

  function handleMessage(event: MessageEvent) {
    const msg = event.data?.pluginMessage;
    if (!msg) return;

    if (msg.type === "ws_config") {
      const prefs = normalizeStoredPrefs(msg.config);
      serverHost = prefs.host;
      serverPort = prefs.port;
      autoCopyEnabled = prefs.autoCopy;
      guardMode = prefs.guardMode;
      if (prefs.showLog !== showLog) {
        showLog = prefs.showLog;
        resizePanel();
      }
      if (!configLoaded) {
        configLoaded = true;
        connect();
      }
      return;
    }

    if (msg.type === "plugin-capabilities") {
      pluginHandlers = msg.handlers ?? [];
      // The socket may already be up; re-announce so the server is not left
      // with the versionless first frame.
      if (socket?.readyState === WebSocket.OPEN) announce(socket);
      return;
    }

    if (msg.type === "plugin-status") {
      fileName = msg.payload.fileName;
      pageName = msg.payload.pageName ?? "—";
      selectionCount = msg.payload.selectionCount;
      selectedNodes = msg.payload.selectedNodes ?? [];
      
      if (autoCopyEnabled && !autoCopyBroken && selectedNodes.length > 0) {
        copyAllNodes(true);
      }
      return;
    }

    if ("requestId" in msg) {
      if (msg.type === "progress_update") {
        activityLog = progressEntry(activityLog, msg.requestId, msg.message);
      } else {
        activityLog = finishEntry(activityLog, msg.requestId, msg.error, Date.now());
      }
      sendToServer(msg);
    }
  }

  // ── Request admission ──────────────────────────────────────────────────────

  function sendToServer(message: any) {
    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify(message));
    }
  }

  /** Answer the server ourselves, without the request ever reaching the core. */
  function refuse(payload: any, reason: string) {
    activityLog = startEntry(activityLog, payload.requestId, payload.type, Date.now());
    activityLog = finishEntry(activityLog, payload.requestId, reason, Date.now());
    sendToServer({ type: payload.type, requestId: payload.requestId, error: reason });
  }

  function forward(payload: any) {
    activityLog = startEntry(activityLog, payload.requestId, payload.type, Date.now());
    parent.postMessage({ pluginMessage: { type: "server-request", payload } }, "*");
  }

  /**
   * Decide what happens to an incoming request.
   *
   * The gate lives here rather than in the plugin core because approving needs
   * a dialog, and only this side has one. There is no trust boundary between
   * the two — both are the plugin — so one gate is enough.
   */
  function admitRequest(payload: any) {
    const mutating = isMutating(payload.type, payload.params);

    if (guardMode === "readonly" && mutating) {
      refuse(payload, `Read-only mode is on in the Figma plugin panel — ${payload.type} was not run`);
      return;
    }

    if (guardMode === "confirm" && isDestructive(payload.type, payload.params)) {
      pendingApprovals = [
        ...pendingApprovals,
        { payload, reason: destructiveReason(payload.type, payload.params) },
      ];
      return;
    }

    forward(payload);
  }

  /**
   * The server has stopped waiting for a request.
   *
   * A request still queued for approval is dropped outright — nobody is left to
   * read its answer, and leaving it in the dialog would ask the user about work
   * that no longer matters. One already running is passed to the core, where a
   * long loop can notice and stop.
   */
  function cancelRequest(requestId: string) {
    const held = pendingApprovals.find(item => item.payload.requestId === requestId);
    if (held) {
      pendingApprovals = pendingApprovals.filter(item => item.payload.requestId !== requestId);
      return;
    }
    parent.postMessage({ pluginMessage: { type: "cancel-request", requestId } }, "*");
    activityLog = finishEntry(activityLog, requestId, "cancelled", Date.now());
  }

  function approvePending() {
    const held = pendingApprovals[0];
    if (!held) return;
    pendingApprovals = pendingApprovals.slice(1);
    forward(held.payload);
  }

  function denyPending() {
    const held = pendingApprovals[0];
    if (!held) return;
    pendingApprovals = pendingApprovals.slice(1);
    refuse(held.payload, `Declined in the Figma plugin panel — ${held.payload.type} was not run`);
  }

  // ── Panel controls ─────────────────────────────────────────────────────────

  function resizePanel() {
    parent.postMessage(
      {
        pluginMessage: {
          type: "resize_ui",
          width: PANEL_WIDTH,
          height: showLog ? PANEL_HEIGHT_WITH_LOG : PANEL_HEIGHT,
        },
      },
      "*",
    );
  }

  function toggleLog() {
    showLog = !showLog;
    resizePanel();
    savePrefs();
  }

  function cycleGuardMode() {
    const index = GUARD_MODES.indexOf(guardMode);
    guardMode = GUARD_MODES[(index + 1) % GUARD_MODES.length];
    // Anything already held under the old mode is no longer being guarded for
    // a reason the user still believes in — let it through rather than leaving
    // the server waiting on a dialog that is gone.
    if (guardMode !== "confirm" && pendingApprovals.length > 0) {
      const held = pendingApprovals;
      pendingApprovals = [];
      for (const item of held) {
        if (guardMode === "readonly" && isMutating(item.payload.type, item.payload.params)) {
          refuse(item.payload, `Read-only mode is on in the Figma plugin panel — ${item.payload.type} was not run`);
        } else {
          forward(item.payload);
        }
      }
    }
    savePrefs();
  }

  function undoLast() {
    parent.postMessage({ pluginMessage: { type: "trigger_undo" } }, "*");
  }

  function copyLog() {
    copyToClipboard(formatLog(activityLog, Date.now()));
  }

  function openSettings() {
    editHost = serverHost;
    editPort = serverPort;
    showSettings = true;
  }

  // Persist via plugin core (figma.clientStorage), since localStorage is
  // unavailable in Figma's data: URL environment. Address and preferences go in
  // one object, so every save writes the whole current state.
  function savePrefs() {
    parent.postMessage(
      {
        pluginMessage: {
          type: "save_ws_config",
          config: {
            host: serverHost,
            port: serverPort,
            autoCopy: autoCopyEnabled,
            guardMode,
            showLog,
          },
        },
      },
      "*"
    );
  }

  function applySettings() {
    serverHost = sanitizeHost(editHost);
    serverPort = sanitizePort(editPort);
    savePrefs();
    showSettings = false;
    // Cancel any pending reconnect and reconnect immediately with the new address.
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    connect();
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === "Enter") applySettings();
    if (event.key === "Escape") showSettings = false;
  }

  async function copyToClipboard(text: string, unattended = false) {
    const result = await copyTextToClipboard(text, {
      socket,
      execCommand: (cmd) => document.execCommand(cmd),
      writeText: (txt) => (navigator.clipboard ? navigator.clipboard.writeText(txt) : Promise.reject("no clipboard API")),
    });

    if (result.success) {
      copyError = false;
      return;
    }

    copyError = true;
    if (unattended) {
      autoCopyEnabled = false;
      autoCopyBroken = true;
    }
  }

  function copyAllNodes(unattended = false) {
    const ids = selectedNodes.map(n => n.id).join("\n");
    copyToClipboard(ids, unattended);
  }

  function retryCopy() {
    copyError = false;
    if (selectedNodes.length > 0) {
      copyAllNodes();
    }
  }

  function reArmAutoCopy() {
    autoCopyBroken = false;
    copyError = false;
    autoCopyEnabled = true;
    savePrefs();
  }

  function onAutoCopyToggle() {
    // Turning it back on clears the sticky break, so the next selection retries.
    if (autoCopyEnabled) autoCopyBroken = false;
    savePrefs();
  }

  onMount(() => {
    window.addEventListener("message", handleMessage);

    // Request stored configs from plugin core
    parent.postMessage({ pluginMessage: { type: "get_ws_config" } }, "*");

    // Fallback: if the plugin core doesn't respond within 500 ms (e.g. during
    // dev / hot-reload without a running core), connect with defaults.
    const fallback = setTimeout(() => {
      if (!configLoaded) {
        configLoaded = true;
        connect();
      }
    }, 500);

    return () => {
      if (tick !== null) clearInterval(tick);
      clearTimeout(fallback);
      window.removeEventListener("message", handleMessage);
      if (reconnectTimer !== null) clearTimeout(reconnectTimer);
      if (socket) socket.close();
    };
  });
</script>

<div class="container">
  <div class="info-section">
    <div class="info-row">
      <span class="info-label">File</span>
      <span class="info-value" title={fileName}>{fileName}</span>
    </div>
    <div class="info-row">
      <span class="info-label">Page</span>
      <span class="info-value" title={pageName}>{pageName}</span>
    </div>
    <div class="info-row">
      <span class="info-label">Selection</span>
      <span class="info-value">{selectionCount} node(s)</span>
    </div>
    {#if selectedNodes.length > 0}
      <div class="node-list">
        <div class="node-list-header">
          <label class="auto-copy-label">
            <input
              type="checkbox"
              bind:checked={autoCopyEnabled}
              on:change={onAutoCopyToggle}
            />
            Auto-copy ID
          </label>
          {#if selectedNodes.length > 1}
            <button class="copy-btn" on:click={() => copyAllNodes()} title="Copy all IDs">Copy All</button>
          {/if}
        </div>
        <div class="node-items">
          {#each selectedNodes as node}
            <div class="node-item">
              <span class="node-name" title="{node.name}">{node.name} <span class="node-id">({node.id})</span></span>
              <button class="copy-btn" on:click={() => copyToClipboard(node.id)} title="Copy ID">Copy</button>
            </div>
          {/each}
        </div>
      </div>
    {/if}
    {#if autoCopyBroken}
      <!-- svelte-ignore a11y-click-events-have-key-events -->
      <!-- svelte-ignore a11y-no-static-element-interactions -->
      <div class="error-banner" on:click={reArmAutoCopy} title="Your browser's clipboard security policy blocks automatic copies without user clicks; connect Go MCP Server for native auto-copy, or click to retry">
        ⚠️ Auto-copy disabled (browser policy). Connect Go MCP Server for native auto-copy, or click to retry.
      </div>
    {:else if copyError}
      <!-- svelte-ignore a11y-click-events-have-key-events -->
      <!-- svelte-ignore a11y-no-static-element-interactions -->
      <div class="error-banner" on:click={retryCopy} title="Browsers require you to click here once after a reload to allow clipboard access">
        ⚠️ Copy failed. Click here to retry.
      </div>
    {/if}
  </div>
  {#if versionMismatch}
    <div class="warn-banner" title={versionMismatchDetail}>
      <span>⚠️ {versionMismatch}</span>
    </div>
  {/if}
  {#if pendingApproval}
    <div class="confirm-banner">
      <div class="confirm-text">
        Allow <strong>{pendingApproval.reason}</strong>?
        {#if pendingApprovals.length > 1}
          <span class="confirm-queue">+{pendingApprovals.length - 1} waiting</span>
        {/if}
      </div>
      <div class="confirm-actions">
        <button class="allow-btn" on:click={approvePending}>Allow</button>
        <button class="deny-btn" on:click={denyPending}>Decline</button>
      </div>
    </div>
  {/if}
  {#if isWorking}
    <div class="working-banner">
      <span class="spinner"></span>
      <span class="working-tool" title={currentTool}>{currentTool}</span>
      <span class="working-time">{formatDuration(runningEntries[0], now)}</span>
      {#if runningEntries.length > 1}
        <span class="working-more">+{runningEntries.length - 1}</span>
      {/if}
    </div>
  {/if}
  {#if showLog}
    <div class="log-panel">
      <div class="log-header">
        <span>Activity</span>
        <button class="copy-btn" on:click={copyLog} title="Copy the log for a bug report">Copy</button>
      </div>
      {#if activityLog.length === 0}
        <div class="log-empty">Nothing yet.</div>
      {:else}
        <div class="log-items">
          {#each activityLog as entry (entry.requestId)}
            <div class="log-item" class:failed={entry.status === "error"}>
              <span class="log-mark" class:ok={entry.status === "ok"} class:err={entry.status === "error"}>
                {entry.status === "ok" ? "✓" : entry.status === "error" ? "✕" : "•"}
              </span>
              <span class="log-tool" title={entry.message || entry.tool}>{entry.tool}</span>
              <span class="log-time">{formatDuration(entry, now)}</span>
            </div>
            {#if entry.message && entry.status === "error"}
              <div class="log-message" title={entry.message}>{entry.message}</div>
            {/if}
          {/each}
        </div>
      {/if}
    </div>
  {/if}
  <div class="footer">
    <!-- Row 0: guards and panel controls -->
    <div class="footer-row">
      <button
        class="mode-btn"
        class:confirm={guardMode === "confirm"}
        class:readonly={guardMode === "readonly"}
        on:click={cycleGuardMode}
        title="Click to change: off runs everything, confirm asks before deletes and bulk rewrites, read-only blocks every change"
      >
        {guardMode === "off" ? "Guard: off" : guardMode === "confirm" ? "Guard: confirm" : "Guard: read-only"}
      </button>
      <div class="links">
        <button class="mini-btn" on:click={undoLast} title="Undo the last change in Figma">Undo</button>
        <button class="mini-btn" class:active={showLog} on:click={toggleLog} title="Show the activity log">
          Log {showLog ? "▴" : "▾"}
        </button>
      </div>
    </div>
    <!-- Row 1: server address (left) + connection badge (right) -->
    <div class="footer-row">
      {#if showSettings}
        <div class="settings-panel">
          <input
            class="addr-input"
            bind:value={editHost}
            placeholder="127.0.0.1"
            on:keydown={handleKeydown}
          />
          <span class="addr-sep">:</span>
          <input
            class="port-input"
            bind:value={editPort}
            placeholder="1994"
            on:keydown={handleKeydown}
          />
          <button class="apply-btn" on:click={applySettings} title="Apply">✓</button>
          <button class="cancel-btn" on:click={() => showSettings = false} title="Cancel">✕</button>
        </div>
      {:else}
        <button
          class="server-addr"
          on:click={openSettings}
          title="Click to configure server address"
        >{serverHost}:{serverPort}</button>
      {/if}
      <div class="badge" class:connected class:disconnected={!connected}>
        <span class="dot" class:connected></span>
        <span>{connected ? (serverVersion ? `Connected (v${serverVersion})` : "Connected") : "Disconnected"}</span>
      </div>
    </div>
    <!-- Row 2: author (left) + bug report + feature suggestion (right) -->
    <div class="footer-row">
      <a
        class="author"
        href="https://github.com/tunglt1810/figma-mcp-go"
        target="_blank"
      >
        <img
          src="https://avatars.githubusercontent.com/u/19906349?v=4"
          alt="avatar"
        />
        tunglt1810
      </a>
      <div class="links">
        <a
          class="footer-link"
          href="https://github.com/tunglt1810/figma-mcp-go/issues/new?labels=bug"
          target="_blank"
          title="Report a bug"
        >
          <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
            <path d="M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0ZM1.5 8a6.5 6.5 0 1 0 13 0 6.5 6.5 0 0 0-13 0Zm7-3.25v2.992l2.028.812.772-1.932-2.8-1.872ZM6.272 3.937 3.5 5.808l.772 1.932L6.3 6.928V3.873a.75.75 0 0 0-.028.064ZM8.75 9.75H7.25V11h1.5V9.75Zm0-5.5H7.25v4h1.5v-4Z"/>
          </svg>
          Bug
        </a>
        <a
          class="footer-link"
          href="https://github.com/tunglt1810/figma-mcp-go/issues/new?labels=enhancement&title=Feature+request%3A+"
          target="_blank"
          title="Suggest a feature"
        >
          <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
            <path d="M8 1.5c-2.363 0-4 1.69-4 3.75 0 .984.424 1.625.984 2.304l.214.253c.223.264.47.556.673.848.284.411.537.896.621 1.49a.75.75 0 0 1-1.484.211c-.04-.282-.163-.547-.37-.847a8.456 8.456 0 0 0-.542-.68c-.084-.1-.173-.205-.268-.32C3.201 7.75 2.5 6.766 2.5 5.25 2.5 2.31 4.863 0 8 0s5.5 2.31 5.5 5.25c0 1.516-.701 2.5-1.328 3.259-.095.115-.184.22-.268.319-.207.245-.383.453-.541.681-.208.3-.33.565-.37.847a.751.751 0 0 1-1.485-.212c.084-.593.337-1.078.621-1.489.203-.292.45-.584.673-.848.075-.088.147-.173.213-.253.561-.679.985-1.32.985-2.304 0-2.06-1.637-3.75-4-3.75ZM5.75 12h4.5a.75.75 0 0 1 0 1.5h-4.5a.75.75 0 0 1 0-1.5ZM6 14.25a.75.75 0 0 1 .75-.75h2.5a.75.75 0 0 1 0 1.5h-2.5a.75.75 0 0 1-.75-.75Z"/>
          </svg>
          Suggest
        </a>
      </div>
    </div>
  </div>
</div>

<style>
  :global(*) {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
  }

  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    font-size: 12px;
    background: #1e1e1e;
    color: #e0e0e0;
    height: 100vh;
  }

  .container {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 16px;
    gap: 12px;
  }

  .info-section {
    display: flex;
    flex-direction: column;
    gap: 8px;
    flex-shrink: 0;
  }

  .info-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .info-label {
    color: #888;
  }

  .info-value {
    color: #e0e0e0;
    font-weight: 500;
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .node-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-top: 4px;
    background: #2a2a2a;
    border-radius: 6px;
    padding: 6px 8px;
    max-height: 120px;
    overflow-y: auto;
  }

  .node-list-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 10px;
    color: #888;
    margin-bottom: 2px;
    padding-bottom: 4px;
    border-bottom: 1px solid #444;
  }

  .auto-copy-label {
    display: flex;
    align-items: center;
    gap: 4px;
    cursor: pointer;
  }
  
  .auto-copy-label input {
    cursor: pointer;
    margin: 0;
  }

  .node-items {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .node-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
  }

  .node-name {
    color: #ccc;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-size: 11px;
    flex: 1;
  }

  .node-id {
    color: #888;
    font-family: monospace;
    font-size: 10px;
  }

  .copy-btn {
    background: #333;
    border: 1px solid #444;
    color: #e0e0e0;
    border-radius: 4px;
    padding: 2px 6px;
    font-size: 10px;
    cursor: pointer;
    flex-shrink: 0;
  }

  .copy-btn:hover {
    background: #444;
  }
  
  .copy-btn:active {
    background: #555;
  }
  
  .node-list::-webkit-scrollbar {
    width: 6px;
  }
  
  .node-list::-webkit-scrollbar-track {
    background: transparent;
  }
  
  .node-list::-webkit-scrollbar-thumb {
    background: #555;
    border-radius: 3px;
  }

  .confirm-banner {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px 10px;
    background: #3a2f1a;
    border: 1px solid #fbbf2466;
    border-radius: 8px;
    color: #fbbf24;
    font-size: 11px;
  }

  .confirm-text {
    line-height: 1.4;
    word-break: break-word;
  }

  .confirm-queue {
    color: #a1a1aa;
    font-size: 10px;
  }

  .confirm-actions {
    display: flex;
    gap: 6px;
  }

  .allow-btn,
  .deny-btn {
    flex: 1;
    border-radius: 4px;
    padding: 4px 8px;
    font-size: 11px;
    font-weight: 600;
    cursor: pointer;
    border: 1px solid #444;
  }

  .allow-btn {
    background: #1a472a;
    border-color: #4ade8055;
    color: #4ade80;
  }

  .allow-btn:hover {
    background: #1f5733;
  }

  .deny-btn {
    background: #3a1a1a;
    border-color: #f8717155;
    color: #f87171;
  }

  .deny-btn:hover {
    background: #4a1a1a;
  }

  .log-panel {
    display: flex;
    flex-direction: column;
    gap: 4px;
    background: #2a2a2a;
    border-radius: 6px;
    padding: 6px 8px;
    flex: 1;
    min-height: 0;
    overflow-y: auto;
  }

  .log-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 10px;
    color: #888;
    padding-bottom: 4px;
    border-bottom: 1px solid #444;
    position: sticky;
    top: -6px;
    background: #2a2a2a;
  }

  .log-empty {
    color: #666;
    font-size: 11px;
    padding: 6px 0;
  }

  .log-items {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .log-item {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
  }

  .log-mark {
    color: #60a5fa;
    flex-shrink: 0;
    width: 10px;
    text-align: center;
  }

  .log-mark.ok {
    color: #4ade80;
  }

  .log-mark.err {
    color: #f87171;
  }

  .log-tool {
    color: #ccc;
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-family: monospace;
    font-size: 10px;
  }

  .log-time {
    color: #777;
    font-size: 10px;
    flex-shrink: 0;
  }

  .log-message {
    color: #f87171;
    font-size: 10px;
    line-height: 1.3;
    padding: 0 0 2px 16px;
    word-break: break-word;
  }

  .log-panel::-webkit-scrollbar {
    width: 6px;
  }

  .log-panel::-webkit-scrollbar-track {
    background: transparent;
  }

  .log-panel::-webkit-scrollbar-thumb {
    background: #555;
    border-radius: 3px;
  }

  .mode-btn {
    background: #2a2a2a;
    border: 1px solid #444;
    color: #888;
    border-radius: 4px;
    padding: 3px 8px;
    font-size: 10px;
    cursor: pointer;
  }

  .mode-btn:hover {
    color: #e0e0e0;
  }

  .mode-btn.confirm {
    color: #fbbf24;
    border-color: #fbbf2455;
  }

  .mode-btn.readonly {
    color: #60a5fa;
    border-color: #60a5fa55;
  }

  .mini-btn {
    background: none;
    border: 1px solid #444;
    color: #888;
    border-radius: 4px;
    padding: 3px 8px;
    font-size: 10px;
    cursor: pointer;
  }

  .mini-btn:hover {
    color: #e0e0e0;
    background: #2a2a2a;
  }

  .mini-btn.active {
    color: #e0e0e0;
    border-color: #666;
  }

  .working-tool {
    font-family: monospace;
    font-size: 10px;
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .working-time,
  .working-more {
    color: #93c5fd;
    font-size: 10px;
    flex-shrink: 0;
  }

  .working-banner {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    background: #1a2e3a;
    border: 1px solid #2563eb44;
    border-radius: 8px;
    color: #60a5fa;
    font-size: 11px;
    font-weight: 500;
  }

  .warn-banner {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    padding: 6px 10px;
    background: #3a2f1a;
    border: 1px solid #fbbf2444;
    border-radius: 8px;
    color: #fbbf24;
    font-size: 10px;
    line-height: 1.4;
    font-weight: 500;
  }

  .error-banner {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    padding: 6px 10px;
    background: #3a1a1a;
    border: 1px solid #f8717144;
    border-radius: 8px;
    color: #f87171;
    font-size: 11px;
    font-weight: 500;
    cursor: pointer;
    text-align: center;
  }

  .error-banner:hover {
    background: #4a1a1a;
  }

  .spinner {
    width: 10px;
    height: 10px;
    border: 2px solid #60a5fa44;
    border-top-color: #60a5fa;
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
    flex-shrink: 0;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .footer {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .footer-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .links {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .footer-link {
    display: flex;
    align-items: center;
    gap: 4px;
    text-decoration: none;
    color: #888;
    font-size: 11px;
  }

  .footer-link:hover {
    color: #e0e0e0;
  }

  .author {
    display: flex;
    align-items: center;
    gap: 6px;
    text-decoration: none;
    color: #888;
    font-size: 11px;
  }

  .author:hover {
    color: #e0e0e0;
  }

  .author img {
    width: 20px;
    height: 20px;
    border-radius: 50%;
  }

  /* Server address button — shows current host:port, click to edit */
  .server-addr {
    background: none;
    border: none;
    color: #666;
    font-size: 10px;
    font-family: monospace;
    cursor: pointer;
    padding: 2px 4px;
    border-radius: 4px;
  }

  .server-addr:hover {
    color: #aaa;
    background: #2a2a2a;
  }

  /* Inline settings panel — takes remaining space so inputs aren't squished */
  .settings-panel {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1;
  }

  .addr-input {
    width: 72px;
    background: #2a2a2a;
    border: 1px solid #444;
    border-radius: 4px;
    color: #e0e0e0;
    font-size: 10px;
    font-family: monospace;
    padding: 2px 4px;
    outline: none;
  }

  .addr-input:focus {
    border-color: #555;
  }

  .port-input {
    width: 36px;
    background: #2a2a2a;
    border: 1px solid #444;
    border-radius: 4px;
    color: #e0e0e0;
    font-size: 10px;
    font-family: monospace;
    padding: 2px 4px;
    outline: none;
  }

  .port-input:focus {
    border-color: #555;
  }

  .addr-sep {
    color: #666;
    font-size: 10px;
  }

  .apply-btn,
  .cancel-btn {
    background: none;
    border: none;
    cursor: pointer;
    font-size: 11px;
    padding: 1px 3px;
    border-radius: 3px;
  }

  .apply-btn {
    color: #4ade80;
  }

  .apply-btn:hover {
    background: #1a3a2a;
  }

  .cancel-btn {
    color: #f87171;
  }

  .cancel-btn:hover {
    background: #3a1a1a;
  }

  .badge {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 12px;
    font-size: 11px;
    font-weight: 600;
  }

  .badge.connected {
    background: #1a472a;
    color: #4ade80;
  }

  .badge.disconnected {
    background: #3a1a1a;
    color: #f87171;
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #f87171;
  }

  .dot.connected {
    background: #4ade80;
  }
</style>
