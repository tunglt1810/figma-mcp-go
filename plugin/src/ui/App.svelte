<script lang="ts">
  import { onMount } from "svelte";
  import { copyTextToClipboard } from "./copy-helper";
  import { versionWarning, versionWarningSummary } from "./version-check";
  import {
    DEFAULT_HOST,
    DEFAULT_PANEL_HEIGHT,
    DEFAULT_PANEL_WIDTH,
    DEFAULT_PORT,
    GUARD_MODES,
    LOG_EXTRA_HEIGHT,
    MAX_PANEL_HEIGHT,
    MAX_PANEL_WIDTH,
    MIN_PANEL_HEIGHT,
    MIN_PANEL_WIDTH,
    normalizeStoredPrefs,
    sanitizeHost,
    sanitizePanelHeight,
    sanitizePanelWidth,
    sanitizePort,
  } from "./prefs";
  import type { GuardMode } from "./prefs";
  import { finishEntry, formatDuration, formatLog, progressEntry, startEntry } from "./activity";
  import type { ActivityEntry } from "./activity";
  import { destructiveReason, isDestructive, isMutating } from "../tool-classes";
  import { pickLocale, strings } from "./i18n";

  // The panel is translated; nothing that leaves it is. Refusal text the server
  // receives and the activity log a user pastes into a bug report stay English —
  // they are read by the MCP client and by whoever the report goes to.
  const locale = pickLocale(typeof navigator !== "undefined" ? navigator.languages : undefined);
  const t = strings(locale);

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

  // The size with the log closed. The log's extra height is added on top, so a
  // panel the user widened stays that width whether the log is open or not.
  let panelWidth = DEFAULT_PANEL_WIDTH;
  let panelHeight = DEFAULT_PANEL_HEIGHT;
  
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
  // Whether the server says its listener is reachable from another machine.
  let serverExposed = false;
  // So the confirm guard is raised once per panel session rather than on every
  // reconnect, which would undo a deliberate choice to turn it off.
  let exposureHandled = false;

  const pluginVersion = __APP_VERSION__;
  // Sent by the plugin core once it starts. It can arrive either side of the
  // socket opening, so both paths announce, and neither assumes the other ran.
  let pluginHandlers: string[] = [];
  // Recomputed whenever the server reports its version — null while
  // disconnected, or when the two versions are close enough not to matter.
  $: versionMismatch = connected ? versionWarningSummary(pluginVersion, serverVersion) : null;
  $: versionMismatchDetail = connected ? versionWarning(pluginVersion, serverVersion) : null;

  // The pinned context set. Held by the plugin core; the panel shows what the
  // core echoes back, never what it hoped it sent.
  let pinnedNodes: { id: string, name: string }[] = [];
  $: pinnedIds = pinnedNodes.map(node => node.id);
  $: selectionIsPinned =
    selectedNodes.length > 0 &&
    selectedNodes.length === pinnedNodes.length &&
    selectedNodes.every(node => pinnedIds.includes(node.id));

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
          serverExposed = payload.exposed === true;
          // The socket carries no authentication — pairing was considered and
          // rejected, because a prompt in front of every connect costs every
          // local user something to protect the few who move the listener off
          // loopback. So when the server says it is reachable from the network,
          // the destructive tools are gated instead of the connection. Only
          // from "off", and only once: a user who has chosen a mode keeps it,
          // and one who turns this back off is not overruled on every frame.
          if (serverExposed && guardMode === "off" && !exposureHandled) {
            exposureHandled = true;
            guardMode = "confirm";
            savePrefs();
          }
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
      const sizeChanged =
        prefs.panelWidth !== panelWidth || prefs.panelHeight !== panelHeight;
      panelWidth = sanitizePanelWidth(prefs.panelWidth);
      panelHeight = sanitizePanelHeight(prefs.panelHeight);
      if (prefs.showLog !== showLog || sizeChanged) {
        showLog = prefs.showLog;
        resizePanel();
      }
      if (!configLoaded) {
        configLoaded = true;
        connect();
      }
      return;
    }

    if (msg.type === "pinned_nodes") {
      const ids: string[] = msg.nodeIds ?? [];
      // Names come from whatever the panel last saw. A pinned node the user has
      // since deselected keeps the name it was pinned under; one the panel never
      // saw shows its id, which is still enough to identify it.
      const known = new Map(
        [...selectedNodes, ...pinnedNodes].map(node => [node.id, node.name]),
      );
      pinnedNodes = ids.map(id => ({ id, name: known.get(id) ?? id }));
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
          width: panelWidth,
          height: showLog ? panelHeight + LOG_EXTRA_HEIGHT : panelHeight,
        },
      },
      "*",
    );
  }

  // Figma plugin windows have no resize handle of their own, so the panel draws
  // one and asks the core to resize. The drag is tracked from the pointer's
  // position on screen rather than from a delta, so a fast drag that outruns the
  // resize cannot make the grip drift away from the cursor.
  let resizing = false;

  function startResize(event: PointerEvent) {
    resizing = true;
    (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
    event.preventDefault();
  }

  function trackResize(event: PointerEvent) {
    if (!resizing) return;
    const width = Math.min(Math.max(event.clientX + 4, MIN_PANEL_WIDTH), MAX_PANEL_WIDTH);
    const height = Math.min(Math.max(event.clientY + 4, MIN_PANEL_HEIGHT), MAX_PANEL_HEIGHT);
    panelWidth = width;
    // What is stored is the closed-log size, so reopening the log does not
    // stack its extra height on a panel that already includes it.
    panelHeight = showLog ? Math.max(height - LOG_EXTRA_HEIGHT, MIN_PANEL_HEIGHT) : height;
    resizePanel();
  }

  function endResize(event: PointerEvent) {
    if (!resizing) return;
    resizing = false;
    (event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId);
    // Written once at the end of the drag rather than on every pointer move —
    // clientStorage is a round trip through the plugin core.
    savePrefs();
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
            panelWidth,
            panelHeight,
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

  // ── Pinned context ─────────────────────────────────────────────────────────

  function pinSelection() {
    parent.postMessage(
      { pluginMessage: { type: "set_pinned_nodes", nodeIds: selectedNodes.map(n => n.id) } },
      "*",
    );
  }

  function clearPin() {
    parent.postMessage({ pluginMessage: { type: "set_pinned_nodes", nodeIds: [] } }, "*");
  }

  function copyPinned() {
    copyToClipboard(pinnedIds.join(", "));
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
    // The pin outlives a panel reload, so ask what the core is already holding.
    parent.postMessage({ pluginMessage: { type: "get_pinned_nodes" } }, "*");

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
      <span class="info-label">{t.file}</span>
      <span class="info-value" title={fileName}>{fileName}</span>
    </div>
    <div class="info-row">
      <span class="info-label">{t.page}</span>
      <span class="info-value" title={pageName}>{pageName}</span>
    </div>
    <div class="info-row">
      <span class="info-label">{t.selection}</span>
      <span class="info-value">{t.nodeCount(selectionCount)}</span>
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
            {t.autoCopyId}
          </label>
          {#if selectedNodes.length > 1}
            <button class="copy-btn" on:click={() => copyAllNodes()} title={t.copyAllTitle}>{t.copyAll}</button>
          {/if}
          <button
            class="copy-btn"
            class:pinned={selectionIsPinned}
            on:click={pinSelection}
            title={t.pinTitle}
          >{selectionIsPinned ? t.pinned : t.pin}</button>
        </div>
        <div class="node-items">
          {#each selectedNodes as node}
            <div class="node-item">
              <span class="node-name" title="{node.name}">{node.name} <span class="node-id">({node.id})</span></span>
              <button class="copy-btn" on:click={() => copyToClipboard(node.id)} title={t.copyIdTitle}>{t.copy}</button>
            </div>
          {/each}
        </div>
      </div>
    {/if}
    {#if pinnedNodes.length > 0}
      <div class="info-row pinned-row">
        <span class="info-label" title={t.pinnedTitle}>{t.pinnedCount(pinnedNodes.length)}</span>
        <span class="pinned-actions">
          <button class="copy-btn" on:click={copyPinned} title={pinnedNodes.map(n => `${n.name} (${n.id})`).join("\n")}>{t.copy}</button>
          <button class="copy-btn" on:click={clearPin} title={t.clearPinTitle}>{t.clear}</button>
        </span>
      </div>
    {/if}
    {#if autoCopyBroken}
      <!-- svelte-ignore a11y-click-events-have-key-events -->
      <!-- svelte-ignore a11y-no-static-element-interactions -->
      <div class="error-banner" on:click={reArmAutoCopy} title={t.autoCopyBrokenTitle}>{t.autoCopyBroken}</div>
    {:else if copyError}
      <!-- svelte-ignore a11y-click-events-have-key-events -->
      <!-- svelte-ignore a11y-no-static-element-interactions -->
      <div class="error-banner" on:click={retryCopy} title={t.copyFailedTitle}>{t.copyFailed}</div>
    {/if}
  </div>
  {#if serverExposed}
    <div class="warn-banner" title={t.exposedTitle}>
      <span>⚠️ {t.exposed}</span>
    </div>
  {/if}
  {#if versionMismatch}
    <div class="warn-banner" title={versionMismatchDetail}>
      <span>⚠️ {versionMismatch}</span>
    </div>
  {/if}
  {#if pendingApproval}
    <div class="confirm-banner">
      <div class="confirm-text">
        {t.allowQuestion} <strong>{pendingApproval.reason}</strong>?
        {#if pendingApprovals.length > 1}
          <span class="confirm-queue">{t.waiting(pendingApprovals.length - 1)}</span>
        {/if}
      </div>
      <div class="confirm-actions">
        <button class="allow-btn" on:click={approvePending}>{t.allow}</button>
        <button class="deny-btn" on:click={denyPending}>{t.decline}</button>
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
        <span>{t.activity}</span>
        <button class="copy-btn" on:click={copyLog} title={t.copyLogTitle}>{t.copy}</button>
      </div>
      {#if activityLog.length === 0}
        <div class="log-empty">{t.logEmpty}</div>
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
        title={t.guardTitle}
      >
        {guardMode === "off" ? t.guardOff : guardMode === "confirm" ? t.guardConfirm : t.guardReadonly}
      </button>
      <div class="links">
        <button class="mini-btn" on:click={undoLast} title={t.undoTitle}>{t.undo}</button>
        <button class="mini-btn" class:active={showLog} on:click={toggleLog} title={t.activityTitle}>
          {t.activity} {showLog ? "▴" : "▾"}
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
          <button class="apply-btn" on:click={applySettings} title={t.apply}>✓</button>
          <button class="cancel-btn" on:click={() => showSettings = false} title={t.dismiss}>✕</button>
        </div>
      {:else}
        <button
          class="server-addr"
          on:click={openSettings}
          title={t.serverAddressTitle}
        >{serverHost}:{serverPort}</button>
      {/if}
      <div class="badge" class:connected class:disconnected={!connected}>
        <span class="dot" class:connected></span>
        <span>{connected ? (serverVersion ? t.connectedVersion(serverVersion) : t.connected) : t.disconnected}</span>
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
          title={t.reportBug}
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
          title={t.suggestFeature}
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

<!-- Figma gives a plugin window no resize handle of its own, so the panel draws
     one. svelte-ignore: the grip is a pointer affordance with no keyboard role;
     the panel is fully usable at any size without it. -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div
  class="resize-grip"
  class:resizing
  title={t.resizeTitle}
  on:pointerdown={startResize}
  on:pointermove={trackResize}
  on:pointerup={endResize}
  on:pointercancel={endResize}
></div>

<style>
  /*
   * Figma puts a `figma-dark` class on the document element in dark mode and
   * removes it in light mode, so the theme is a CSS question and the panel needs
   * no script for it. Light is the base and dark is the override — the panel was
   * dark-only before this, so the dark block is the palette it already had.
   */
  :global(:root) {
    --bg: #ffffff;
    --bg-raised: #f5f5f5;
    --bg-sunken: #e8e8e8;
    --text: #333333;
    --text-muted: #6b6b6b;
    --text-faint: #949494;
    --border: #d5d5d5;
    --border-strong: #bcbcbc;

    --ok: #15803d;
    --ok-ring: #15803d55;
    --ok-bg: #dcfce7;
    --ok-bg-strong: #bbf7d0;
    --ok-bg-soft: #ecfdf3;

    --danger: #b91c1c;
    --danger-ring: #b91c1c55;
    --danger-ring-soft: #b91c1c33;
    --danger-bg: #fee2e2;
    --danger-bg-strong: #fecaca;

    --warn: #a16207;
    --warn-ring: #a1620766;
    --warn-ring-soft: #a1620744;
    --warn-ring-faint: #a1620733;
    --warn-bg: #fef3c7;

    --accent: #1d4ed8;
    --accent-bright: #1e40af;
    --accent-ring: #1d4ed855;
    --accent-ring-soft: #1d4ed844;
    --accent-ring-faint: #1d4ed833;
    --accent-bg: #dbeafe;
  }

  :global(.figma-dark) {
    --bg: #1e1e1e;
    --bg-raised: #2a2a2a;
    --bg-sunken: #333;
    --text: #e0e0e0;
    --text-muted: #888;
    --text-faint: #666;
    --border: #444;
    --border-strong: #555;

    --ok: #4ade80;
    --ok-ring: #4ade8055;
    --ok-bg: #1a472a;
    --ok-bg-strong: #1f5733;
    --ok-bg-soft: #1a3a2a;

    --danger: #f87171;
    --danger-ring: #f8717155;
    --danger-ring-soft: #f8717144;
    --danger-bg: #3a1a1a;
    --danger-bg-strong: #4a1a1a;

    --warn: #fbbf24;
    --warn-ring: #fbbf2466;
    --warn-ring-soft: #fbbf2455;
    --warn-ring-faint: #fbbf2444;
    --warn-bg: #3a2f1a;

    --accent: #60a5fa;
    --accent-bright: #93c5fd;
    --accent-ring: #60a5fa55;
    --accent-ring-soft: #60a5fa44;
    --accent-ring-faint: #2563eb44;
    --accent-bg: #1a2e3a;
  }

  :global(*) {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
  }

  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    font-size: 12px;
    background: var(--bg);
    color: var(--text);
    height: 100vh;
  }

  .container {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 16px;
    gap: 12px;
  }

  .pinned-row {
    padding-top: 2px;
  }

  .pinned-actions {
    display: flex;
    gap: 4px;
  }

  .copy-btn.pinned {
    border-color: var(--accent);
    color: var(--accent);
  }

  .resize-grip {
    position: fixed;
    right: 0;
    bottom: 0;
    width: 16px;
    height: 16px;
    cursor: nwse-resize;
    touch-action: none;
    /* Two short strokes in the corner, the same shape Figma's own resizable
       surfaces use, drawn in CSS so there is no image to load. */
    background:
      linear-gradient(135deg, transparent 50%, var(--border-strong) 50%, var(--border-strong) 62%, transparent 62%),
      linear-gradient(135deg, transparent 74%, var(--border-strong) 74%, var(--border-strong) 86%, transparent 86%);
  }

  .resize-grip:hover,
  .resize-grip.resizing {
    background:
      linear-gradient(135deg, transparent 50%, var(--text-muted) 50%, var(--text-muted) 62%, transparent 62%),
      linear-gradient(135deg, transparent 74%, var(--text-muted) 74%, var(--text-muted) 86%, transparent 86%);
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
    color: var(--text-muted);
  }

  .info-value {
    color: var(--text);
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
    background: var(--bg-raised);
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
    color: var(--text-muted);
    margin-bottom: 2px;
    padding-bottom: 4px;
    border-bottom: 1px solid var(--border);
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
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-size: 11px;
    flex: 1;
  }

  .node-id {
    color: var(--text-muted);
    font-family: monospace;
    font-size: 10px;
  }

  .copy-btn {
    background: var(--bg-sunken);
    border: 1px solid var(--border);
    color: var(--text);
    border-radius: 4px;
    padding: 2px 6px;
    font-size: 10px;
    cursor: pointer;
    flex-shrink: 0;
  }

  .copy-btn:hover {
    background: var(--border);
  }
  
  .copy-btn:active {
    background: var(--border-strong);
  }
  
  .node-list::-webkit-scrollbar {
    width: 6px;
  }
  
  .node-list::-webkit-scrollbar-track {
    background: transparent;
  }
  
  .node-list::-webkit-scrollbar-thumb {
    background: var(--border-strong);
    border-radius: 3px;
  }

  .confirm-banner {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px 10px;
    background: var(--warn-bg);
    border: 1px solid var(--warn-ring);
    border-radius: 8px;
    color: var(--warn);
    font-size: 11px;
  }

  .confirm-text {
    line-height: 1.4;
    word-break: break-word;
  }

  .confirm-queue {
    color: var(--text-muted);
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
    border: 1px solid var(--border);
  }

  .allow-btn {
    background: var(--ok-bg);
    border-color: var(--ok-ring);
    color: var(--ok);
  }

  .allow-btn:hover {
    background: var(--ok-bg-strong);
  }

  .deny-btn {
    background: var(--danger-bg);
    border-color: var(--danger-ring);
    color: var(--danger);
  }

  .deny-btn:hover {
    background: var(--danger-bg-strong);
  }

  .log-panel {
    display: flex;
    flex-direction: column;
    gap: 4px;
    background: var(--bg-raised);
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
    color: var(--text-muted);
    padding-bottom: 4px;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: -6px;
    background: var(--bg-raised);
  }

  .log-empty {
    color: var(--text-faint);
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
    color: var(--accent);
    flex-shrink: 0;
    width: 10px;
    text-align: center;
  }

  .log-mark.ok {
    color: var(--ok);
  }

  .log-mark.err {
    color: var(--danger);
  }

  .log-tool {
    color: var(--text);
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-family: monospace;
    font-size: 10px;
  }

  .log-time {
    color: var(--text-faint);
    font-size: 10px;
    flex-shrink: 0;
  }

  .log-message {
    color: var(--danger);
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
    background: var(--border-strong);
    border-radius: 3px;
  }

  .mode-btn {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    color: var(--text-muted);
    border-radius: 4px;
    padding: 3px 8px;
    font-size: 10px;
    cursor: pointer;
  }

  .mode-btn:hover {
    color: var(--text);
  }

  .mode-btn.confirm {
    color: var(--warn);
    border-color: var(--warn-ring-soft);
  }

  .mode-btn.readonly {
    color: var(--accent);
    border-color: var(--accent-ring);
  }

  .mini-btn {
    background: none;
    border: 1px solid var(--border);
    color: var(--text-muted);
    border-radius: 4px;
    padding: 3px 8px;
    font-size: 10px;
    cursor: pointer;
  }

  .mini-btn:hover {
    color: var(--text);
    background: var(--bg-raised);
  }

  .mini-btn.active {
    color: var(--text);
    border-color: var(--text-faint);
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
    color: var(--accent-bright);
    font-size: 10px;
    flex-shrink: 0;
  }

  .working-banner {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    background: var(--accent-bg);
    border: 1px solid var(--accent-ring-faint);
    border-radius: 8px;
    color: var(--accent);
    font-size: 11px;
    font-weight: 500;
  }

  .warn-banner {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    padding: 6px 10px;
    background: var(--warn-bg);
    border: 1px solid var(--warn-ring-faint);
    border-radius: 8px;
    color: var(--warn);
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
    background: var(--danger-bg);
    border: 1px solid var(--danger-ring-soft);
    border-radius: 8px;
    color: var(--danger);
    font-size: 11px;
    font-weight: 500;
    cursor: pointer;
    text-align: center;
  }

  .error-banner:hover {
    background: var(--danger-bg-strong);
  }

  .spinner {
    width: 10px;
    height: 10px;
    border: 2px solid var(--accent-ring-soft);
    border-top-color: var(--accent);
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
    color: var(--text-muted);
    font-size: 11px;
  }

  .footer-link:hover {
    color: var(--text);
  }

  .author {
    display: flex;
    align-items: center;
    gap: 6px;
    text-decoration: none;
    color: var(--text-muted);
    font-size: 11px;
  }

  .author:hover {
    color: var(--text);
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
    color: var(--text-faint);
    font-size: 10px;
    font-family: monospace;
    cursor: pointer;
    padding: 2px 4px;
    border-radius: 4px;
  }

  .server-addr:hover {
    color: var(--text-muted);
    background: var(--bg-raised);
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
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text);
    font-size: 10px;
    font-family: monospace;
    padding: 2px 4px;
    outline: none;
  }

  .addr-input:focus {
    border-color: var(--border-strong);
  }

  .port-input {
    width: 36px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text);
    font-size: 10px;
    font-family: monospace;
    padding: 2px 4px;
    outline: none;
  }

  .port-input:focus {
    border-color: var(--border-strong);
  }

  .addr-sep {
    color: var(--text-faint);
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
    color: var(--ok);
  }

  .apply-btn:hover {
    background: var(--ok-bg-soft);
  }

  .cancel-btn {
    color: var(--danger);
  }

  .cancel-btn:hover {
    background: var(--danger-bg);
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
    background: var(--ok-bg);
    color: var(--ok);
  }

  .badge.disconnected {
    background: var(--danger-bg);
    color: var(--danger);
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--danger);
  }

  .dot.connected {
    background: var(--ok);
  }
</style>
