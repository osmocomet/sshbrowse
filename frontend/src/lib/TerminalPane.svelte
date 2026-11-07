<script module lang="ts">
  import { Terminal } from "@xterm/xterm";
  import { SearchSelectionCopyGuard } from "./terminalSearch";

  class SearchableTerminal extends Terminal {
    readonly searchSelectionCopy = new SearchSelectionCopyGuard();

    override select(column: number, row: number, length: number) {
      // xterm fires selection changes synchronously. The addon also selects
      // during delayed output refreshes, which a guard around findNext misses.
      // Mouse selection and selectAll do not use this programmatic select API.
      if (!this.searchSelectionCopy.preservingSelection) {
        this.searchSelectionCopy.run(() => super.select(column, row, length));
      }
    }

    override clearSelection() {
      if (!this.searchSelectionCopy.preservingSelection) {
        super.clearSelection();
      }
    }
  }

  // Process-instance ids are chosen here, before Go starts the process, so this
  // pane is already listening when the first output event arrives. Unique per app run.
  let nextProcessInstanceId = 1;
</script>

<script lang="ts">
  import { onMount, tick } from "svelte";
  import { FitAddon } from "@xterm/addon-fit";
  import type { ISearchResultChangeEvent } from "@xterm/addon-search";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import ChevronUp from "@lucide/svelte/icons/chevron-up";
  import X from "@lucide/svelte/icons/x";
  import "@xterm/xterm/css/xterm.css";
  import { Clipboard, Events } from "@wailsio/runtime";
  import * as Sessions from "../../bindings/sshbrowse/internal/app/sessions";
  import type {
    FileDropEvent,
    SessionData,
    SessionExit,
    SessionLoggingState,
  } from "../../bindings/sshbrowse/internal/app/models";
  import {
    currentPlatform,
    linuxShortcutFor,
    windowsShortcutFor,
    terminalFontCommandFor,
    terminalFontEvent,
    type TerminalFontCommand,
    type TerminalFontEventDetail,
    tabCommandFor,
    terminalSearchShortcutFor,
  } from "./shortcuts";
  import {
    editMenuEvent,
    isEditMenuAction,
    terminalCopyMenuEvent,
    terminalPasteMenuEvent,
    recordingRequestEvent,
    type RecordingRequest,
    type EditMenuAction,
  } from "./menuEvents";
  import type { Connection } from "../../bindings/sshbrowse/internal/profile/models";
  import type { SessionCommand, SessionStatus } from "./tabs";
  import TerminalPasteConfirmDialog from "./TerminalPasteConfirmDialog.svelte";
  import { defaultCustomPalette, type CustomPalette } from "./customPalette";
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
  } from "./terminalInput";
  import {
    liveBroadcastPasteGenerationError,
    pasteClipboardForBroadcastGeneration,
    type LiveBroadcastPasteState,
  } from "./liveBroadcast";
  import { canApplyLoggingResult, type SessionRecordingControl } from "./sessionLogging";
  import { errorMessage } from "./errors";
  import { normalizeTerminalScrollback } from "./storage";
  import {
    terminalSearchCommandFor,
    TerminalSearchSession,
    terminalSearchQueryLimit,
    terminalSearchResultLabel,
  } from "./terminalSearch";

  const fileDropEvent = "window:filesDropped";

  let {
    sessionId,
    sessionLabel,
    connection,
    command,
    selected,
    autofocus,
    focusRequest,
    searchRequest = 0,
    shortcutsEnabled,
    themeName,
    customPalette = defaultCustomPalette,
    terminalColors,
    terminalFontName,
    defaultFontSize,
    rightClickToPaste,
    copyOnSelection,
    scrollbackLines = 1000,
    onstatus,
    onprocesschange,
    onloggingcontrol,
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
    oncloseerror,
    onsearchopen = () => {},
  }: {
    sessionId: number;
    sessionLabel: string;
    connection: Connection | null;
    command: SessionCommand;
    selected: boolean;
    autofocus: boolean;
    focusRequest: number;
    searchRequest?: number;
    shortcutsEnabled: boolean;
    themeName: ThemeName;
    customPalette?: CustomPalette;
    terminalColors: TerminalColors;
    terminalFontName: TerminalFontName;
    defaultFontSize: number;
    rightClickToPaste: boolean;
    copyOnSelection: boolean;
    scrollbackLines?: number;
    onstatus: (status: SessionStatus) => void;
    onprocesschange: (processInstanceId: number | null) => void;
    onloggingcontrol: (control: SessionRecordingControl) => void;
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
    onpasteconfirmationchange: (confirming: boolean, dismiss?: () => void) => void;
    onpastewarningschange: (disabled: boolean) => void;
    onfocus: () => void;
    onclose: () => void;
    oncloseerror: (sessionLabel: string, logPath: string, message: string) => void;
    onsearchopen?: () => void;
  } = $props();

  // Name of the program in status messages.
  let programName = $derived(connection === null ? "shell" : command);

  let container: HTMLDivElement;
  let terminal: SearchableTerminal;
  let fit: FitAddon;
  let search: TerminalSearchSession | undefined;
  let searchInput = $state<HTMLInputElement | undefined>(undefined);
  let searchOpen = $state(false);
  let searchQuery = $state("");
  let searchCaseSensitive = $state(false);
  let searchError = $state("");
  let searchResult = $state<ISearchResultChangeEvent>({ resultIndex: -1, resultCount: 0 });
  let searchResultLabel = $derived(terminalSearchResultLabel(searchQuery, searchResult));
  let processInstanceId = $state<number | null>(null);
  let loggingProcessId: number | null = null;
  let loggingState = $state<SessionLoggingState | null>(null);
  let loggingActionError = $state("");
  let loggingBusy = $state(false);
  let loggingRevision = 0;
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
  let observedSearchRequest = 0;
  let dropTargetId = $derived(processInstanceId === null ? undefined : `terminal-drop-${processInstanceId}`);
  let dropError = $state("");
  let pasteError = $state("");
  let pasteConfirmation = $state<{
    sessionId: number;
    processInstanceId: number | null;
    broadcastGeneration: number;
    broadcastRecipientCount: number | null;
    text: string;
  } | null>(null);
  const shortcutPlatform = currentPlatform();

  function clampFontSize(size: number): number {
    return Math.min(Math.max(size, 8), 32);
  }

  async function focusSearch(selectQuery = false) {
    await tick();
    if (disposed || !selected || !shortcutsEnabled || !searchOpen) {
      return;
    }
    searchInput?.focus();
    search?.refresh();
    if (selectQuery) {
      searchInput?.select();
    }
  }

  function createSearchSession() {
    search = new TerminalSearchSession(terminal, terminal.searchSelectionCopy, {
      isQueryFocused: () => selected && shortcutsEnabled && document.hasFocus()
        && document.activeElement === searchInput,
      onResults: (result) => {
        if (!disposed && searchOpen) {
          searchResult = result;
        }
      },
      onError: (message) => {
        if (!disposed && searchOpen) {
          searchError = message;
        }
      },
    });
  }

  function openSearch() {
    onsearchopen();
    searchOpen = true;
    if (search === undefined) {
      createSearchSession();
      findSearchMatch("next", true);
    } else if (searchError) {
      findSearchMatch("next", true);
    }
    void focusSearch(true);
  }

  function closeSearch() {
    searchOpen = false;
    // Disposal also cancels pending output refreshes before ordinary selection resumes.
    search?.dispose();
    search = undefined;
    terminal.clearSelection();
    searchResult = { resultIndex: -1, resultCount: 0 };
    searchError = "";
    if (selected && shortcutsEnabled) {
      terminal.focus();
    }
  }

  function findSearchMatch(direction: "next" | "previous", incremental = false) {
    if (search === undefined) {
      return;
    }
    search.find(searchQuery, searchCaseSensitive, direction, incremental);
  }

  function updateSearch() {
    findSearchMatch("next", true);
  }

  function handleSearchInput(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    // Edit-menu paste uses setRangeText, which bypasses the browser's maxlength.
    const query = input.value.slice(0, terminalSearchQueryLimit);
    if (input.value !== query) {
      input.value = query;
    }
    searchQuery = query;
    updateSearch();
  }

  function navigateSearch(direction: "next" | "previous") {
    findSearchMatch(direction);
    void focusSearch();
  }

  function handleSearchKeydown(event: KeyboardEvent) {
    const command = terminalSearchCommandFor(event);
    if (command === null || (command !== "close" && event.target !== searchInput)) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    if (command === "close") {
      closeSearch();
    } else {
      navigateSearch(command);
    }
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
    };
    onpasteconfirmationchange(true, () => finishPasteConfirmation(null, false));
  }

  function finishPasteConfirmation(text: string | null, disableWarnings: boolean) {
    const request = pasteConfirmation;
    pasteConfirmation = null;
    onpasteconfirmationchange(false);
    if (text === null || text.length === 0 || request === null) {
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
    terminal.paste(text);
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

  function applyLoggingState(next: SessionLoggingState) {
    loggingRevision++;
    loggingState = {
      id: next.id,
      active: next.active && next.error === "" && status !== "closed",
      path: next.path,
      error: next.error,
    };
    loggingActionError = "";
  }

  async function setLogging(start: boolean) {
    const id = processInstanceId;
    if (disposed || id === null || status !== "live" || loggingBusy) {
      return;
    }
    if (start === (loggingState?.active === true)) {
      return;
    }
    const stopping = !start;
    const revision = loggingRevision;
    loggingBusy = true;
    loggingActionError = "";
    try {
      const next = stopping ? await Sessions.StopLogging(id) : await Sessions.StartLogging(id, terminal.buffer.active.cursorX);
      if (!disposed && canApplyLoggingResult(next.id, loggingProcessId, revision, loggingRevision)) {
        applyLoggingState(next);
      }
    } catch (error) {
      if (!disposed && loggingProcessId === id && loggingRevision === revision) {
        loggingActionError = `Could not ${stopping ? "stop" : "start"} session log: ${errorMessage(error)}`;
      }
    } finally {
      if (!disposed && loggingProcessId === id) {
        loggingBusy = false;
      }
    }
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
    loggingProcessId = newProcessInstanceId;
    loggingRevision++;
    loggingState = null;
    loggingActionError = "";
    loggingBusy = false;
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
      if (disposed || processInstanceId !== newProcessInstanceId) {
        // Ignore failures from a process that has exited or been replaced.
        return;
      }
      processInstanceId = null;
      onprocesschange(null);
      terminal.write(`\r\n[failed to start ${programName}: ${err}]\r\n`);
      setStatus("closed");
      return;
    }
    if (disposed || processInstanceId !== newProcessInstanceId) {
      // A pane closed or reconnected while Open was pending.
      closeSession(newProcessInstanceId, loggingPathForProcess(newProcessInstanceId));
      return;
    }
    setStatus("live");
    // A fit while Open was pending was not sent to the new process.
    requestResize(terminal.cols, terminal.rows);
    if (focusWhenReady && selected && !searchOpen) {
      terminal.focus();
    }
  }

  // Scrollback is kept; the new session starts below the old output.
  function reconnect() {
    closedInput = "";
    terminal.write("\r\n");
    connect(true);
  }

  function loggingPathForProcess(id: number): string {
    return loggingState?.id === id ? loggingState.path : "";
  }

  function closeSession(id: number, logPath: string) {
    const closingSessionLabel = sessionLabel;
    void Sessions.Close(id).catch((error: unknown) => {
      oncloseerror(closingSessionLabel, logPath, errorMessage(error));
    });
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
    terminal = new SearchableTerminal({
      fontFamily: terminalFontFamilyFor(terminalFontName),
      fontSize,
      cursorBlink: true,
      macOptionIsMeta: false,
      theme: terminalThemeFor(themeName, terminalColors, customPalette),
      minimumContrastRatio: terminalMinimumContrastRatio(themeName, terminalColors),
      scrollback: normalizeTerminalScrollback(scrollbackLines),
      // The official search addon uses xterm's proposed decoration API.
      allowProposedApi: true,
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
      if (terminalSearchShortcutFor(event)) {
        event.preventDefault();
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
    // The active element survives window blur, so refocus may not fire on the input.
    const onWindowFocus = () => search?.refresh();
    window.addEventListener("focus", onWindowFocus);

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
        const finalLoggingState = loggingState;
        if (finalLoggingState !== null && finalLoggingState.id === event.data.id && finalLoggingState.active) {
          // The process has ended; do not leave the pane claiming that its
          // recorder is still active if the final state event is delayed.
          loggingRevision++;
          loggingState = { ...finalLoggingState, active: false };
        }
        exitCode = event.data.exitCode;
        terminal.write(`\r\n[${programName} exited with code ${exitCode}]\r\n`);
        setStatus("closed");
      }
    });
    const offLogging = Events.On("session:logging", (event: { data: SessionLoggingState }) => {
      if (!disposed && event.data.id === loggingProcessId) {
        applyLoggingState(event.data);
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
      if (copyOnSelection && !terminal.searchSelectionCopy.active) {
        copyTerminalSelection();
      }
    });
    terminal.onResize(({ cols, rows }) => {
      requestResize(cols, rows);
    });

    // Hidden panes are still laid out at full size, so every session tracks the window.
    const observer = new ResizeObserver(() => fit.fit());
    observer.observe(container);

    const onRecordingRequest = (event: Event) => {
      const request = (event as CustomEvent<RecordingRequest>).detail;
      if (shortcutsEnabled && request.target === `${sessionId}:${processInstanceId}`) {
        void setLogging(request.start);
      }
    };
    window.addEventListener(recordingRequestEvent, onRecordingRequest);
    connect(autofocus);

    return () => {
      disposed = true;
      observer.disconnect();
      offData();
      offExit();
      offLogging();
      window.removeEventListener(recordingRequestEvent, onRecordingRequest);
      offFileDrop();
      window.removeEventListener(terminalFontEvent, onTerminalFontCommand);
      window.removeEventListener("focus", onWindowFocus);
      offEditMenu();
      offTerminalCopy();
      offTerminalPaste();
      search?.dispose();
      search = undefined;
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
        closeSession(
          closingProcessInstanceId,
          loggingPathForProcess(closingProcessInstanceId),
        );
      }
      terminal.dispose();
    };
  });

  $effect(() => {
    const focusWasRequested = focusRequest !== observedFocusRequest;
    if (mounted && selected && (!wasSelected || focusWasRequested)) {
      fit.fit();
      if (searchOpen) {
        void focusSearch();
      } else {
        terminal.focus();
      }
    }
    wasSelected = selected;
    observedFocusRequest = focusRequest;
  });

  $effect(() => {
    const request = searchRequest;
    if (!mounted) {
      return;
    }
    if (request > 0 && request !== observedSearchRequest && selected && shortcutsEnabled) {
      openSearch();
    }
    observedSearchRequest = request;
  });

  $effect(() => {
    const scrollback = normalizeTerminalScrollback(scrollbackLines);
    if (!mounted || terminal.options.scrollback === scrollback) {
      return;
    }
    terminal.options.scrollback = scrollback;
    // This trim does not emit xterm's public write or resize events.
    search?.invalidate();
  });

  $effect(() => {
    const nextThemeName = themeName;
    const nextTerminalColors = terminalColors;
    const nextPalette = customPalette;
    const nextFontName = terminalFontName;
    const nextDefaultFontSize = clampFontSize(defaultFontSize);
    if (!mounted) {
      return;
    }

    terminal.options.theme = terminalThemeFor(nextThemeName, nextTerminalColors, nextPalette);
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
  $effect(() => {
    const available = status === "live" && processInstanceId !== null && !loggingBusy;
    onloggingcontrol({
      indicator: loggingIndicator,
      active: loggingState?.active === true,
      target: `${sessionId}:${processInstanceId}`,
      canStart: available && loggingState?.active !== true,
      canStop: available && loggingState?.active === true,
    });
  });
</script>

{#snippet loggingIndicator()}
  {#if loggingState?.active}
    <span
      class="recording-indicator"
      role="status"
      aria-label={`Session log active for ${sessionLabel}`}
      title={`Logging terminal output\n${loggingState.path}\nStop from Shell or the session's right-click menu.`}
    ><span class="recording-dot" aria-hidden="true"></span><span aria-hidden="true">LOG</span></span>
  {/if}
{/snippet}

<div
  id={dropTargetId}
  class="pane"
  data-terminal-session-id={sessionId}
  data-file-drop-target={status === "live" && shortcutsEnabled ? "" : undefined}
  onfocusin={onfocus}
  onfocusout={() => onfocusout(sessionId)}
>
  <div class="terminal-area">
    {#if loggingState?.error || loggingActionError}
      <div class="logging-error" role="alert">
        {loggingActionError || (loggingState?.error ? `Logging failed: ${loggingState.error}` : "")}
      </div>
    {/if}
    <div
      class="term"
      bind:this={container}
      role="region"
      aria-label="Terminal"
      style:--custom-contextmenu="terminal"
      style:--custom-contextmenu-data={sessionId}
      oncontextmenu={handleTerminalContextMenu}
    ></div>
    {#if searchOpen}
      <form class="terminal-search" role="search" aria-label="Search terminal output" onsubmit={(event) => event.preventDefault()}>
        <input
          bind:this={searchInput}
          bind:value={searchQuery}
          aria-label="Find in terminal"
          placeholder="Find in terminal"
          maxlength={terminalSearchQueryLimit}
          autocomplete="off"
          spellcheck="false"
          onkeydown={handleSearchKeydown}
          oninput={handleSearchInput}
          onfocus={() => search?.refresh()}
          onblur={() => search?.clearActiveDecoration()}
        />
        <span class="search-results" role="status" aria-live="polite">{searchError || searchResultLabel}</span>
        <button type="button" class:active={searchCaseSensitive} aria-label="Match case" aria-pressed={searchCaseSensitive} title="Match case" onkeydown={handleSearchKeydown} onclick={() => { searchCaseSensitive = !searchCaseSensitive; updateSearch(); }}>Aa</button>
        <button type="button" aria-label="Previous match" title="Previous match (Shift+Enter)" disabled={!searchQuery || searchResult.resultCount === 0} onkeydown={handleSearchKeydown} onclick={() => navigateSearch("previous")}><ChevronUp size={15} /></button>
        <button type="button" aria-label="Next match" title="Next match (Enter)" disabled={!searchQuery || searchResult.resultCount === 0} onkeydown={handleSearchKeydown} onclick={() => navigateSearch("next")}><ChevronDown size={15} /></button>
        <button type="button" aria-label="Close search" title="Close search (Escape)" onkeydown={handleSearchKeydown} onclick={closeSearch}><X size={15} /></button>
      </form>
    {/if}
  </div>
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
      <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users can focus the status to read exit details.) -->
      <span class="closed-status" tabindex={exitCode !== null ? 0 : undefined} aria-describedby={exitCode !== null ? `${sessionId}-exit-code` : undefined}>
        {connection === null ? "Shell closed" : "Disconnected"}
        {#if exitCode !== null}
          <span class="exit-tooltip" id={`${sessionId}-exit-code`} role="tooltip">Exit code: {exitCode}</span>
        {/if}
      </span>
      <button onclick={reconnect}>{connection === null ? "Restart" : "Reconnect"} ⏎</button>
    </div>
  {/if}
</div>

{#if pasteConfirmation}
  <TerminalPasteConfirmDialog
    text={pasteConfirmation.text}
    sessionLabel={sessionLabel}
    recipientCount={pasteConfirmation.broadcastRecipientCount}
    onclose={finishPasteConfirmation}
  />
{/if}

<style>
  .pane {
    position: relative;
    display: flex;
    flex-direction: column;
    flex: 1;
    width: 100%;
    min-width: 0;
    min-height: 0;
    background: var(--terminal-background);
  }
  .term {
    position: relative;
    flex: 1;
    width: 100%;
    min-height: 0;
    overflow: hidden;
    background: var(--terminal-background);
  }
  .terminal-area {
    position: relative;
    display: flex;
    flex: 1;
    min-width: 0;
    min-height: 0;
  }
  .recording-indicator {
    display: inline-flex;
    flex: none;
    align-items: center;
    gap: 4px;
    color: var(--status-error);
    font: 600 var(--ui-font-tiny) var(--font-ui);
    white-space: nowrap;
  }
  .recording-dot {
    width: 6px;
    height: 6px;
    flex: none;
    border-radius: 50%;
    background: currentColor;
  }
  .logging-error {
    position: absolute;
    z-index: 2;
    top: 8px;
    right: 8px;
    max-width: min(520px, calc(100% - 16px));
    box-sizing: border-box;
    padding: 4px 8px;
    border: 1px solid var(--status-error);
    border-radius: var(--radius-control);
    background: var(--status-error-surface);
    color: var(--status-error);
    font: var(--ui-font-small) var(--font-ui);
    overflow-wrap: anywhere;
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
  .terminal-search {
    position: absolute;
    z-index: 3;
    top: 8px;
    right: 8px;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px;
    max-width: calc(100% - 16px);
    padding: 6px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--surface-raised);
    color: var(--text-primary);
    font: var(--ui-font-small) var(--font-ui);
    box-shadow: var(--shadow-panel);
  }
  .terminal-search input {
    flex: 1;
    min-width: 60px;
    width: 160px;
    height: 28px;
    padding: 0 7px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--input-surface);
    color: var(--text-primary);
    font: inherit;
  }
  .search-results {
    min-width: 72px;
    padding: 0 3px;
    color: var(--text-secondary);
    text-align: center;
  }
  .terminal-search button {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    padding: 0;
    border: 1px solid transparent;
    border-radius: var(--radius-control);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
  }
  .terminal-search button:hover:not(:disabled) {
    background: var(--control-hover);
    color: var(--text-primary);
  }
  .terminal-search button.active {
    border-color: var(--accent);
    color: var(--accent);
  }
  .terminal-search button:disabled {
    opacity: 0.4;
  }
  .terminal-search input:focus,
  .terminal-search button:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 1px;
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
  .closed-status {
    position: relative;
    white-space: nowrap;
  }
  .closed-status:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 3px;
  }
  .exit-tooltip {
    display: none;
    position: absolute;
    bottom: 100%;
    left: 0;
    padding: 6px 9px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--surface-raised);
    color: var(--text-secondary);
    box-shadow: var(--shadow-panel);
  }
  .closed-status:hover .exit-tooltip,
  .closed-status:focus .exit-tooltip {
    display: block;
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
