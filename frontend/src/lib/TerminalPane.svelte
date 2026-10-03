<script module lang="ts">
  // Process-instance ids are chosen here, before Go starts the process, so this
  // pane is already listening when the first output event arrives. Unique per app run.
  let nextProcessInstanceId = 1;
</script>

<script lang="ts">
  import { onMount } from "svelte";
  import { Terminal } from "@xterm/xterm";
  import { FitAddon } from "@xterm/addon-fit";
  import "@xterm/xterm/css/xterm.css";
  import { Clipboard, Events } from "@wailsio/runtime";
  import * as Sessions from "../../bindings/sshbrowse/internal/app/sessions";
  import type { FileDropEvent, SessionData, SessionExit } from "../../bindings/sshbrowse/internal/app/models";
  import {
    currentPlatform,
    linuxShortcutFor,
    windowsShortcutFor,
    terminalFontCommandFor,
    terminalFontEvent,
    type TerminalFontCommand,
    type TerminalFontEventDetail,
    tabCommandFor,
  } from "./shortcuts";
  import {
    editMenuEvent,
    isEditMenuAction,
    terminalCopyMenuEvent,
    terminalPasteMenuEvent,
    type EditMenuAction,
  } from "./menuEvents";
  import type { Connection } from "../../bindings/sshbrowse/internal/profile/models";
  import type { SessionCommand, SessionStatus } from "./tabs";
  import TerminalPasteConfirmDialog from "./TerminalPasteConfirmDialog.svelte";
  import { terminalFontFamilyFor, terminalMinimumContrastRatio, terminalThemeFor, type TerminalColors, type TerminalFontName, type ThemeName } from "./appearance";
  import {
    classifyTerminalPaste,
    formatDroppedPaths,
    handleMacTerminalEditing,
    handleShiftEnter,
    isTerminalTargetCurrent,
    splitTerminalInput,
    terminalRightClickAction,
    type TerminalInputEvent,
    type TerminalPasteInfo,
  } from "./terminalInput";
  import {
    liveBroadcastPasteGenerationError,
    pasteClipboardForBroadcastGeneration,
    type LiveBroadcastPasteState,
  } from "./liveBroadcast";

  const fileDropEvent = "window:filesDropped";

  let {
    sessionId,
    sessionLabel,
    connection,
    command,
    selected,
    autofocus,
    focusRequest,
    shortcutsEnabled,
    themeName,
    terminalColors,
    terminalFontName,
    defaultFontSize,
    rightClickToPaste,
    copyOnSelection,
    onstatus,
    onprocesschange,
    oninput,
    onfocusout,
    onfont,
    getBroadcastPasteState,
    pasteWarningsDisabled,
    validatepaste,
    onpasteconfirmationchange,
    onpastewarningschange,
    onfocus,
    onclose,
  }: {
    sessionId: number;
    sessionLabel: string;
    connection: Connection | null;
    command: SessionCommand;
    selected: boolean;
    autofocus: boolean;
    focusRequest: number;
    shortcutsEnabled: boolean;
    themeName: ThemeName;
    terminalColors: TerminalColors;
    terminalFontName: TerminalFontName;
    defaultFontSize: number;
    rightClickToPaste: boolean;
    copyOnSelection: boolean;
    onstatus: (status: SessionStatus) => void;
    onprocesschange: (processInstanceId: number | null) => void;
    oninput: (event: TerminalInputEvent) => void;
    onfocusout: (sessionId: number) => void;
    onfont: (command: TerminalFontCommand) => void;
    getBroadcastPasteState: () => LiveBroadcastPasteState;
    pasteWarningsDisabled: boolean;
    validatepaste: (
      sessionId: number,
      processInstanceId: number | null,
      broadcastGeneration: number,
      broadcastRecipientCount: number | null,
    ) => string | null;
    onpasteconfirmationchange: (confirming: boolean) => void;
    onpastewarningschange: (disabled: boolean) => void;
    onfocus: () => void;
    onclose: () => void;
  } = $props();

  // Name of the program in status messages.
  let programName = $derived(connection === null ? "shell" : command);

  let container: HTMLDivElement;
  let terminal: Terminal;
  let fit: FitAddon;
  let processInstanceId = $state<number | null>(null);
  let disposed = false;
  let status = $state<SessionStatus>("connecting");
  let exitCode = $state<number | null>(null);
  let fontSize = $state(14);
  // Per-pane shortcuts pin the size until reset; global changes update unpinned panes.
  let fontSizeManuallyOverridden = false;
  let appliedFontName: TerminalFontName | null = null;
  let pendingFitFrame: number | null = null;
  let pendingResize: { id: number; cols: number; rows: number } | null = null;
  let resizeInProgress = false;
  let mounted = $state(false);
  let wasSelected = false;
  let observedFocusRequest = 0;
  let dropTargetId = $derived(processInstanceId === null ? undefined : `terminal-drop-${processInstanceId}`);
  let dropError = $state("");
  let pasteError = $state("");
  let pasteConfirmation = $state<({
    sessionId: number;
    processInstanceId: number | null;
    broadcastGeneration: number;
    broadcastRecipientCount: number | null;
    text: string;
  } & TerminalPasteInfo) | null>(null);
  const shortcutPlatform = currentPlatform();

  function clampFontSize(size: number): number {
    return Math.min(Math.max(size, 8), 32);
  }

  function copyTerminalSelection() {
    if (!terminal.hasSelection()) {
      return;
    }
    const selection = terminal.getSelection();
    if (selection.length === 0) {
      return;
    }
    Clipboard.SetText(selection).catch((err) => {
      console.warn("Could not copy terminal selection to the clipboard", err);
    });
  }

  function terminalHasFocus(): boolean {
    return terminal.element?.contains(document.activeElement) ?? false;
  }

  function pasteTerminalClipboard(requireFocus = true) {
    const targetProcessInstanceId = processInstanceId;
    pasteClipboardForBroadcastGeneration(
      () => Clipboard.Text(),
      getBroadcastPasteState,
      (text, targetBroadcastState) => {
        if (
          disposed ||
          targetProcessInstanceId === null ||
          processInstanceId !== targetProcessInstanceId ||
          status !== "live" ||
          (requireFocus && !terminalHasFocus())
        ) {
          return;
        }
        requestTerminalPaste(text, targetBroadcastState);
      },
      (capturedState, currentState) => {
        if (!disposed) {
          pasteError = liveBroadcastPasteGenerationError(capturedState.generation, currentState.generation) ?? "";
        }
      },
    )
      .catch((err) => console.warn("Could not read the clipboard for terminal paste", err));
  }

  function requestTerminalPaste(
    text: string,
    targetBroadcastState = getBroadcastPasteState(),
  ) {
    if (disposed || pasteConfirmation !== null) {
      return;
    }
    const validationError = validatepaste(
      sessionId,
      processInstanceId,
      targetBroadcastState.generation,
      targetBroadcastState.recipientCount,
    );
    if (validationError !== null) {
      pasteError = validationError;
      return;
    }
    const pasteInfo = classifyTerminalPaste(text);
    if (!pasteInfo.requiresConfirmation || pasteWarningsDisabled) {
      // Use xterm's paste path so bracketed paste and line-ending
      // normalization stay identical to an ordinary paste event.
      terminal.paste(text);
      return;
    }
    pasteError = "";
    pasteConfirmation = {
      sessionId,
      processInstanceId,
      broadcastGeneration: targetBroadcastState.generation,
      broadcastRecipientCount: targetBroadcastState.recipientCount,
      text,
      ...pasteInfo,
    };
    onpasteconfirmationchange(true);
  }

  function finishPasteConfirmation(confirmed: boolean, disableWarnings: boolean) {
    const request = pasteConfirmation;
    pasteConfirmation = null;
    onpasteconfirmationchange(false);
    if (!confirmed || request === null) {
      return;
    }
    if (!isTerminalTargetCurrent(request, sessionId, processInstanceId)) {
      pasteError = "The terminal changed before the paste was confirmed. Nothing was pasted.";
      return;
    }
    const broadcastValidationError = validatepaste(
      request.sessionId,
      request.processInstanceId,
      request.broadcastGeneration,
      request.broadcastRecipientCount,
    );
    if (broadcastValidationError !== null) {
      pasteError = broadcastValidationError;
      return;
    }
    if (disableWarnings) {
      onpastewarningschange(true);
    }
    // Keep the approved text on xterm's paste path so bracketed paste and
    // terminal line-ending behavior remain intact.
    terminal.paste(request.text);
  }

  function handleTerminalContextMenu(event: MouseEvent) {
    onfocus();
    const action = terminalRightClickAction(
      terminal.modes.mouseTrackingMode,
      event.shiftKey,
      rightClickToPaste,
      status === "live" && processInstanceId !== null,
    );
    if (action === "menu") {
      return;
    }

    // Wails discovers custom context menus from this inherited CSS variable.
    // Temporarily hide it when the click belongs to the terminal or paste.
    const element = event.currentTarget as HTMLElement;
    const menu = element.style.getPropertyValue("--custom-contextmenu");
    const data = element.style.getPropertyValue("--custom-contextmenu-data");
    element.style.removeProperty("--custom-contextmenu");
    element.style.removeProperty("--custom-contextmenu-data");
    window.setTimeout(() => {
      if (!disposed) {
        element.style.setProperty("--custom-contextmenu", menu);
        element.style.setProperty("--custom-contextmenu-data", data);
      }
    }, 0);
    if (action === "paste") {
      event.preventDefault();
      terminal.focus();
      pasteTerminalClipboard(false);
    }
  }

  function handleEditMenuAction(action: EditMenuAction) {
    if (!terminalHasFocus()) {
      return;
    }
    switch (action) {
      case "copy":
        copyTerminalSelection();
        break;
      case "paste":
        pasteTerminalClipboard();
        break;
      case "selectAll":
        terminal.selectAll();
        break;
      case "cut":
      case "undo":
      case "redo":
        // Terminal output is not an editable document. These Edit actions
        // are handled by form controls when one has focus instead.
        break;
    }
  }

  function scheduleFit() {
    if (pendingFitFrame !== null) {
      cancelAnimationFrame(pendingFitFrame);
    }
    pendingFitFrame = requestAnimationFrame(() => {
      pendingFitFrame = null;
      if (!disposed) {
        fit.fit();
      }
    });
  }

  function changeFontSize(command: TerminalFontCommand) {
    let nextFontSize = fontSize;
    switch (command) {
      case "increase":
        nextFontSize = Math.min(fontSize + 1, 32);
        break;
      case "decrease":
        nextFontSize = Math.max(fontSize - 1, 8);
        break;
      case "reset":
        nextFontSize = clampFontSize(defaultFontSize);
        break;
    }
    if (command === "reset") {
      fontSizeManuallyOverridden = false;
    }
    if (nextFontSize === fontSize) {
      return;
    }
    if (command !== "reset") {
      fontSizeManuallyOverridden = true;
    }
    fontSize = nextFontSize;
    terminal.options.fontSize = nextFontSize;
    scheduleFit();
  }

  function setStatus(next: SessionStatus) {
    status = next;
    onstatus(next);
  }

  function requestResize(cols: number, rows: number) {
    if (processInstanceId === null || status !== "live") {
      return;
    }
    pendingResize = { id: processInstanceId, cols, rows };
    if (!resizeInProgress) {
      void flushResize();
    }
  }

  async function flushResize() {
    resizeInProgress = true;
    try {
      while (pendingResize !== null) {
        const resize = pendingResize;
        pendingResize = null;
        try {
          await Sessions.Resize(resize.id, resize.cols, resize.rows);
        } catch (err) {
          if (processInstanceId === resize.id && status === "live") {
            console.error(err);
          }
        }
      }
    } finally {
      resizeInProgress = false;
    }
  }

  function base64ToBytes(encoded: string): Uint8Array {
    const binary = atob(encoded);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i);
    }
    return bytes;
  }

  async function connect(focusWhenReady: boolean) {
    setStatus("connecting");
    exitCode = null;
    const newProcessInstanceId = nextProcessInstanceId++;
    processInstanceId = newProcessInstanceId;
    onprocesschange(newProcessInstanceId);
    try {
      if (connection === null) {
        await Sessions.OpenLocal(newProcessInstanceId, terminal.cols, terminal.rows);
      } else if (command === "sftp") {
        await Sessions.OpenSFTP(newProcessInstanceId, $state.snapshot(connection), terminal.cols, terminal.rows);
      } else {
        await Sessions.Open(newProcessInstanceId, $state.snapshot(connection), terminal.cols, terminal.rows);
      }
    } catch (err) {
      if (disposed) {
        return;
      }
      if (processInstanceId === newProcessInstanceId) {
        processInstanceId = null;
        onprocesschange(null);
      }
      terminal.write(`\r\n[failed to start ${programName}: ${err}]\r\n`);
      setStatus("closed");
      return;
    }
    if (disposed) {
      // The pane was closed while the process was starting; nobody owns it now.
      if (processInstanceId === newProcessInstanceId) {
        processInstanceId = null;
        onprocesschange(null);
      }
      Sessions.Close(newProcessInstanceId).catch(console.error);
      return;
    }
    if (processInstanceId !== newProcessInstanceId) {
      // The process exited while the Open call was returning.
      return;
    }
    setStatus("live");
    // A fit while Open was pending was not sent to the new process.
    requestResize(terminal.cols, terminal.rows);
    if (focusWhenReady && selected) {
      terminal.focus();
    }
  }

  // Scrollback is kept; the new session starts below the old output.
  function reconnect() {
    closedInput = "";
    terminal.write("\r\n");
    connect(true);
  }

  // Text typed into a closed pane, so "exit" dismisses it like a real shell would.
  let closedInput = "";

  function handleClosedInput(input: string) {
    switch (input) {
      case "\x03": // Ctrl+C
      case "\x04": // Ctrl+D
        onclose();
        return;
      case "\r":
        if (closedInput === "") {
          reconnect();
          return;
        }
        if (["exit", "q", "quit"].includes(closedInput.trim().toLowerCase())) {
          onclose();
          return;
        }
        closedInput = "";
        terminal.write("\r\n");
        return;
      case "\x7f": // Backspace
        if (closedInput.length > 0) {
          closedInput = closedInput.slice(0, -1);
          terminal.write("\b \b");
        }
        return;
    }
    // Echo printable text; control and escape sequences have nothing to act on.
    if (input >= " " && !input.startsWith("\x1b")) {
      closedInput += input;
      terminal.write(input);
    }
  }

  onMount(() => {
    fontSize = clampFontSize(defaultFontSize);
    appliedFontName = terminalFontName;
    terminal = new Terminal({
      fontFamily: terminalFontFamilyFor(terminalFontName),
      fontSize,
      cursorBlink: true,
      macOptionIsMeta: true,
      theme: terminalThemeFor(themeName, terminalColors),
      minimumContrastRatio: terminalMinimumContrastRatio(themeName, terminalColors),
    });
    fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(container);
    fit.fit();
    const interceptTerminalPaste = (event: ClipboardEvent) => {
      const text = event.clipboardData?.getData("text");
      if (text === undefined) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      requestTerminalPaste(text);
    };
    // xterm's own paste handler is on the terminal element. Capture first so
    // risky browser pastes can wait for confirmation before xterm sees them.
    container.addEventListener("paste", interceptTerminalPaste, true);
    wasSelected = selected;
    observedFocusRequest = focusRequest;
    mounted = true;

    // Let tab shortcuts bubble up to the window handler instead of reaching ssh.
    terminal.attachCustomKeyEventHandler((event) => {
      if (linuxShortcutFor(event) !== null || windowsShortcutFor(event) !== null) {
        event.preventDefault();
        return false;
      }
      if (tabCommandFor(event) !== null) {
        return false;
      }
      if (!handleShiftEnter(event, (input) => terminal.input(input))) {
        return false;
      }
      const command = terminalFontCommandFor(event);
      if (command !== null) {
        event.preventDefault();
        if (event.type === "keydown" && selected && shortcutsEnabled) {
          onfont(command);
        }
        return false;
      }

      if (
        selected &&
        shortcutsEnabled &&
        status === "live" &&
        terminalHasFocus() &&
        !handleMacTerminalEditing(event, (input) => terminal.input(input), shortcutPlatform, connection === null)
      ) {
        return false;
      }
      return true;
    });

    const onTerminalFontCommand = (event: Event) => {
      const { command, sessionIds } = (event as CustomEvent<TerminalFontEventDetail>).detail;
      if (shortcutsEnabled && (sessionIds.length > 0 ? sessionIds.includes(sessionId) : selected)) {
        changeFontSize(command);
      }
    };
    window.addEventListener(terminalFontEvent, onTerminalFontCommand);

    const offEditMenu = Events.On(editMenuEvent, (event: { data: unknown }) => {
      if (isEditMenuAction(event.data)) {
        handleEditMenuAction(event.data);
      }
    });
    const offTerminalCopy = Events.On(terminalCopyMenuEvent, (event: { data: unknown }) => {
      if (!disposed && String(event.data) === String(sessionId)) {
        copyTerminalSelection();
      }
    });
    const offTerminalPaste = Events.On(terminalPasteMenuEvent, (event: { data: unknown }) => {
      if (!disposed && String(event.data) === String(sessionId)) {
        onfocus();
        terminal.focus();
        pasteTerminalClipboard(false);
      }
    });

    // Wails hands the payload in `event.data`.
    const offData = Events.On("session:data", (event: { data: SessionData }) => {
      if (event.data.id === processInstanceId) {
        const { id, sequence, data } = event.data;
        terminal.write(base64ToBytes(data), () => {
          if (!disposed && processInstanceId === id) {
            Sessions.AcknowledgeOutput(id, sequence).catch((err) => {
              if (!disposed && processInstanceId === id) {
                console.error(err);
              }
            });
          }
        });
      }
    });
    const offExit = Events.On("session:exit", (event: { data: SessionExit }) => {
      if (event.data.id === processInstanceId) {
        if (pendingResize?.id === event.data.id) {
          pendingResize = null;
        }
        processInstanceId = null;
        onprocesschange(null);
        exitCode = event.data.exitCode;
        terminal.write(`\r\n[${programName} exited with code ${exitCode}]\r\n`);
        setStatus("closed");
      }
    });
    const offFileDrop = Events.On(
      fileDropEvent,
      (event: { data: FileDropEvent }) => {
        if (
          disposed ||
          event.data.targetId !== dropTargetId ||
          processInstanceId === null ||
          status !== "live" ||
          !shortcutsEnabled
        ) {
          return;
        }
        try {
          const syntax = command === "sftp"
            ? "sftp"
            : connection === null && shortcutPlatform === "windows"
              ? "powershell"
              : "shell";
          const input = formatDroppedPaths(event.data.files ?? [], syntax);
          if (input.length === 0) {
            return;
          }
          dropError = "";
          onfocus();
          terminal.focus();
          terminal.paste(input);
        } catch (err) {
          dropError = err instanceof Error ? err.message : String(err);
        }
      },
    );

    function routeTerminalInput(input: string, mirrorToBroadcast = true) {
      if (processInstanceId !== null && status === "live") {
        oninput({ sessionId, processInstanceId, input, mirrorToBroadcast });
      } else if (status === "closed") {
        handleClosedInput(input);
      }
    }

    terminal.onData((input) => {
      for (const segment of splitTerminalInput(input)) {
        routeTerminalInput(segment.input, segment.mirrorToBroadcast);
      }
    });
    terminal.onSelectionChange(() => {
      if (copyOnSelection) {
        copyTerminalSelection();
      }
    });
    terminal.onResize(({ cols, rows }) => {
      requestResize(cols, rows);
    });

    // Hidden panes are still laid out at full size, so every session tracks the window.
    const observer = new ResizeObserver(() => fit.fit());
    observer.observe(container);

    connect(autofocus);

    return () => {
      disposed = true;
      observer.disconnect();
      offData();
      offExit();
      offFileDrop();
      window.removeEventListener(terminalFontEvent, onTerminalFontCommand);
      offEditMenu();
      offTerminalCopy();
      offTerminalPaste();
      container.removeEventListener("paste", interceptTerminalPaste, true);
      if (pendingFitFrame !== null) {
        cancelAnimationFrame(pendingFitFrame);
        pendingFitFrame = null;
      }
      const closingProcessInstanceId = processInstanceId;
      if (pasteConfirmation !== null) {
        onpasteconfirmationchange(false);
      }
      processInstanceId = null;
      onprocesschange(null);
      pendingResize = null;
      if (closingProcessInstanceId !== null && status === "live") {
        Sessions.Close(closingProcessInstanceId).catch(console.error);
      }
      terminal.dispose();
    };
  });

  $effect(() => {
    const focusWasRequested = focusRequest !== observedFocusRequest;
    if (mounted && selected && (!wasSelected || focusWasRequested)) {
      fit.fit();
      terminal.focus();
    }
    wasSelected = selected;
    observedFocusRequest = focusRequest;
  });

  $effect(() => {
    const nextThemeName = themeName;
    const nextTerminalColors = terminalColors;
    const nextFontName = terminalFontName;
    const nextDefaultFontSize = clampFontSize(defaultFontSize);
    if (!mounted) {
      return;
    }

    terminal.options.theme = terminalThemeFor(nextThemeName, nextTerminalColors);
    // xterm adjusts foreground glyphs without turning ANSI black backgrounds gray.
    terminal.options.minimumContrastRatio = terminalMinimumContrastRatio(nextThemeName, nextTerminalColors);
    if (appliedFontName !== nextFontName) {
      terminal.options.fontFamily = terminalFontFamilyFor(nextFontName);
      appliedFontName = nextFontName;
      scheduleFit();
    }
    if (!fontSizeManuallyOverridden && fontSize !== nextDefaultFontSize) {
      fontSize = nextDefaultFontSize;
      terminal.options.fontSize = nextDefaultFontSize;
      scheduleFit();
    }
  });
</script>

<div
  id={dropTargetId}
  class="pane"
  data-terminal-session-id={sessionId}
  data-file-drop-target={status === "live" && shortcutsEnabled ? "" : undefined}
  onfocusin={onfocus}
  onfocusout={() => onfocusout(sessionId)}
>
  <div
    class="term"
    bind:this={container}
    role="region"
    aria-label="Terminal"
    style:--custom-contextmenu="terminal"
    style:--custom-contextmenu-data={sessionId}
    oncontextmenu={handleTerminalContextMenu}
  ></div>
  {#if dropError}
    <div class="banner" role="alert">
      <span>{dropError}</span>
      <button onclick={() => (dropError = "")}>Dismiss</button>
    </div>
  {/if}
  {#if pasteError}
    <div class="banner" role="alert">
      <span>{pasteError}</span>
      <button onclick={() => (pasteError = "")}>Dismiss</button>
    </div>
  {/if}
  {#if status === "closed"}
    <div class="banner">
      <span>{connection === null ? "Shell closed" : "Disconnected"}{exitCode !== null ? `, exit ${exitCode}` : ""}</span>
      <small>exit ⏎ or ^D closes</small>
      <button onclick={reconnect}>{connection === null ? "Restart" : "Reconnect"} ⏎</button>
    </div>
  {/if}
</div>

{#if pasteConfirmation}
  <TerminalPasteConfirmDialog
    lineCount={pasteConfirmation.lineCount}
    byteCount={pasteConfirmation.byteCount}
    sessionLabel={sessionLabel}
    recipientCount={pasteConfirmation.broadcastRecipientCount}
    preview={pasteConfirmation.preview}
    onclose={finishPasteConfirmation}
  />
{/if}

<style>
  .pane {
    position: relative;
    flex: 1;
    width: 100%;
    min-width: 0;
    min-height: 0;
    background: var(--terminal-background);
  }
  .term {
    position: absolute;
    inset: 0;
    overflow: hidden;
    background: var(--terminal-background);
  }
  .term :global(.xterm) {
    /* FitAddon subtracts padding on xterm itself when sizing the grid. */
    padding: 8px;
    width: 100%;
    height: 100%;
    overflow: hidden;
    background: var(--terminal-background);
  }
  .term :global(.xterm-viewport) {
    background: var(--terminal-background);
  }
  .banner {
    position: absolute;
    left: 50%;
    bottom: 16px;
    transform: translateX(-50%);
    display: flex;
    gap: 12px;
    align-items: center;
    padding: 9px 14px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--surface-raised);
    color: var(--text-secondary);
    font: var(--ui-font-small) var(--font-ui);
    box-shadow: var(--shadow-panel);
  }
  .banner small {
    color: var(--text-muted);
  }
  .banner button {
    border: 0;
    border-radius: 6px;
    padding: 4px 10px;
    background: var(--accent);
    color: var(--accent-foreground);
    font: inherit;
  }
</style>
