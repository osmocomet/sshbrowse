<script lang="ts">
  import { onMount } from "svelte";
  import Bookmark from "@lucide/svelte/icons/bookmark";
  import Check from "@lucide/svelte/icons/check";
  import LayoutGrid from "@lucide/svelte/icons/layout-grid";
  import PanelLeft from "@lucide/svelte/icons/panel-left";
  import Radio from "@lucide/svelte/icons/radio";
  import Ungroup from "@lucide/svelte/icons/ungroup";
  import X from "@lucide/svelte/icons/x";
  import { Browser, Clipboard, Events, Window as WailsWindow } from "@wailsio/runtime";
  import * as Sessions from "../bindings/sshbrowse/internal/app/sessions";
  import type { Connection } from "../bindings/sshbrowse/internal/profile/models";
  import Sidebar from "./lib/Sidebar.svelte";
  import TabBar from "./lib/TabBar.svelte";
  import DesktopHeader from "./lib/DesktopHeader.svelte";
  import MoveDialog from "./lib/MoveDialog.svelte";
  import FolderDialog from "./lib/FolderDialog.svelte";
  import BroadcastBar from "./lib/BroadcastBar.svelte";
  import ConnectionForm from "./lib/ConnectionForm.svelte";
  import QuickOpen from "./lib/QuickOpen.svelte";
  import SSHConfigImport from "./lib/SSHConfigImport.svelte";
  import CloseConfirmDialog from "./lib/CloseConfirmDialog.svelte";
  import SettingsPage from "./lib/SettingsPage.svelte";
  import UpdateNotice from "./lib/UpdateNotice.svelte";
  import { canCheckForUpdates, claimStartupUpdateCheck, githubReleaseURL, type UpdateCheckResult, type UpdateRelease } from "./lib/updates";
  import type { TerminalColors, TerminalFontName, ThemeName } from "./lib/appearance";
  import {
    currentPlatform,
    pageApplicationShortcutFor,
    shortcutLabel,
    terminalFontCommandFor,
    terminalFontEvent,
    type ApplicationShortcut,
    type TerminalFontCommand,
    type TerminalFontEventDetail,
    tabCommandFor,
  } from "./lib/shortcuts";
  import {
    maximumTabs,
    sessionDisplayLabel,
    sessionDisplayLabels,
    sessionIds,
    sessionStatusLabel,
    type Session,
    type SessionCommand,
    type Tab,
  } from "./lib/tabs";
  import {
    aboutMenuEvent,
    checkForUpdatesMenuEvent,
    editMenuEvent,
    isEditMenuAction,
    updateDownloadRequestEvent,
    updateInfoEvent,
    updateInfoRequestEvent,
    updateRestartRequestEvent,
    startupUpdateCheckEvent,
    updateCheckResultEvent,
    updateCheckStartedEvent,
    type UpdateInfo,
    type EditMenuAction,
  } from "./lib/menuEvents";
  import { createBroadcastSender, placeNewSession, type BroadcastSnapshot } from "./lib/routing";
  import {
    createLiveBroadcastWriter,
    liveBroadcastPasteError,
    liveBroadcastPasteGenerationError,
    liveBroadcastSnapshotInvalidationReason,
    liveBroadcastStartError,
    type LiveBroadcastPasteState,
    type LiveBroadcastState,
    type LiveBroadcastWriter,
  } from "./lib/liveBroadcast";
  import { isTerminalTargetCurrent, type TerminalInputEvent } from "./lib/terminalInput";
  import {
    connections,
    folders,
    loadConnections,
    saveConnection,
    deleteConnection,
    deleteConnections,
    moveConnections,
    reorderConnections,
    createFolder,
    renameFolder,
    deleteFolder,
    emptyConnection,
    folderNames,
    type ConnectionFormMode,
  } from "./lib/connections.svelte";
  import { folderBaseName, joinFolderPath, type FolderAction } from "./lib/folderTree";
  import { scanSSHConfig, type SSHConfigScan } from "./lib/sshconfig";
  import { tileGrid } from "./lib/tileLayout";
  import { errorMessage } from "./lib/errors";
  import {
    loadPreferences,
    preferenceKeys,
    savePreference,
    interfaceScaleDefault,
    interfaceScaleMaximum,
    interfaceScaleMinimum,
    interfaceScaleStep,
    sidebarDefaultWidth,
    sidebarMinimumWidth,
    type PreferenceKey,
    type UiSize,
  } from "./lib/storage";
  import {
    closeSession as closeSessionState,
    closeTab as closeTabState,
    closeTarget as closeTargetState,
    closeConfirmationFor,
    detachSession as detachSessionState,
    focusedSessionId,
    sessionsForCloseTarget as sessionsForCloseTargetState,
    selectSession as selectSessionState,
    selectTabAt,
    stepTab,
    tileTab as tileTabState,
    type CloseTarget,
    type CloseConfirmationRequest,
    type WorkspaceState,
  } from "./lib/workspace";

  type AppSession = Session & { autofocus: boolean };
  type TerminalPaneComponent = typeof import("./lib/TerminalPane.svelte")["default"];

  let sessions = $state<AppSession[]>([]);
  let tabs = $state<Tab[]>([]);
  let activeId = $state<number | null>(null);
  let nextTabId = 1;
  let nextSessionId = 1;
  let activeTab = $derived(tabs.find((tab) => tab.id === activeId) ?? null);
  let activeWorkspace = $derived(activeTab?.kind === "workspace" ? activeTab : null);
  let activeTileGrid = $derived(activeWorkspace ? tileGrid(activeWorkspace.sessionIds.length) : null);
  let displayLabels = $derived(sessionDisplayLabels(sessions));

  // At most one modal dialog at a time. The menu handlers below enforce that.
  let editing = $state<{ connection: Connection; mode: ConnectionFormMode } | null>(null);
  let moving = $state<Connection[] | null>(null);
  let folderAction = $state<FolderAction | null>(null);
  let picking = $state(false);
  let pickerTilingMode = false;
  let importing = $state<{ scan: SSHConfigScan; firstRun: boolean; initialError?: string } | null>(null);
  let connectionDialog = $state<{ dismiss: () => void }>();
  let moveDialog = $state<{ dismiss: () => void }>();
  let folderDialog = $state<{ dismiss: () => void }>();
  let quickOpenDialog = $state<{ dismiss: () => void }>();
  let importDialog = $state<{ dismiss: () => void }>();
  let closeConfirmationDialog = $state<{ dismiss: () => void }>();
  let closeConfirmation = $state<CloseConfirmationRequest | null>(null);
  let settingsOpen = $state(false);
  let scanningImport = $state(false);
  let copiedConnection = $state<Connection | null>(null);
  // When the form was opened to bookmark an unsaved session, that session adopts the result.
  let bookmarkingSessionId: number | null = null;
  let sidebarVisible = $state(true);
  let sidebarWidth = $state(sidebarDefaultWidth);
  let tilingMode = $state(false);
  let rightClickToPaste = $state(false);
  let copyOnSelection = $state(true);
  let themeName = $state<ThemeName>("classic");
  let terminalColors = $state<TerminalColors>("follow");
  let uiSize = $state<UiSize>("standard");
  let terminalFontSize = $state(14);
  let terminalFontName = $state<TerminalFontName>("system");
  let broadcastBarOpen = $state(false);
  let broadcastSending = $state(false);
  let broadcastConfirming = $state(false);
  let terminalPasteConfirmingSessionId = $state<number | null>(null);
  let terminalPasteWarningsDisabled = $state(false);
  let interfaceScale = $state(interfaceScaleDefault);
  // Menus can take focus before invoking zoom, so remember the last content area.
  let zoomTarget: "terminal" | "sidebar" = "terminal";
  let liveBroadcast = $state<LiveBroadcastState | null>(null);
  let liveBroadcastGeneration = $state(0);
  let liveBroadcastError = $state("");
  let liveBroadcastWriter: LiveBroadcastWriter | null = null;
  const broadcastSender = createBroadcastSender(Sessions.Write);
  let terminalFocusRequest = $state(0);
  let terminalPaneComponent = $state<TerminalPaneComponent | null>(null);
  let terminalPaneLoadError = $state("");
  let terminalPaneLoadStarted = $state(false);
  let sidebarToggleButton = $state<HTMLButtonElement>();
  let operationError = $state("");
  let updateInfo = $state<UpdateInfo | null>(null);
  let updateStatus = $state("");
  let updateError = $state(false);
  let updateBusy = $state(false);
  let updateAvailable = $state(false);
  let updateReady = $state(false);
  let checkUpdatesOnStartup = $state(true);
  let updateReleaseURL = $state("");
  let updatePrompt = $state<UpdateRelease | null>(null);
  let startupCheckScheduled = false;
  let startupPromptWanted = true;
  let dialogReturnFocus: HTMLElement | null = null;
  const shortcutPlatform = currentPlatform();
  const macWindowChrome = shortcutPlatform === "mac";
  const linuxWindowChrome = shortcutPlatform === "linux";
  const desktopMenu = shortcutPlatform !== "mac";
  const newTabShortcut = shortcutLabel("⌘T", "Ctrl+Shift+N", shortcutPlatform);
  const newConnectionShortcut = shortcutLabel("⌘N", "Ctrl+Alt+N", shortcutPlatform);
  const newLocalShortcut = shortcutLabel("⌘⇧T", "Ctrl+Shift+T", shortcutPlatform);
  const newSFTPShortcut = shortcutLabel("⌘⇧P", "Ctrl+Shift+P", shortcutPlatform);
  const closeTabShortcut = shortcutLabel("⌘W", "Ctrl+Shift+W", shortcutPlatform);
  const sidebarShortcut = shortcutLabel("⌘B", "Ctrl+Shift+B", shortcutPlatform);
  const fontShortcuts = {
    increase: shortcutLabel("⌘+", "Ctrl++", shortcutPlatform),
    decrease: shortcutLabel("⌘-", "Ctrl+-", shortcutPlatform),
    reset: shortcutLabel("⌘0", "Ctrl+0", shortcutPlatform),
  };
  const minimiseShortcut = shortcutLabel("⌘M", "Ctrl+Alt+M", shortcutPlatform);
  const quitShortcut = shortcutLabel("⌘Q", "Ctrl+Alt+Q", shortcutPlatform);

  function loadTerminalPane() {
    if (terminalPaneComponent !== null || terminalPaneLoadStarted) {
      return;
    }
    terminalPaneLoadStarted = true;
    terminalPaneLoadError = "";
    import("./lib/TerminalPane.svelte")
      .then(({ default: component }) => {
        terminalPaneComponent = component;
      })
      .catch((error) => {
        console.error("Could not load the terminal pane", error);
        terminalPaneLoadError = "Could not load the terminal. Try again to reopen this session.";
      });
  }

  function retryTerminalPaneLoad() {
    terminalPaneLoadStarted = false;
    loadTerminalPane();
  }

  $effect(() => {
    if (sessions.length === 0) {
      terminalPaneLoadStarted = false;
      terminalPaneLoadError = "";
      return;
    }
    loadTerminalPane();
  });

  function rememberDialogFocus() {
    const activeElement = document.activeElement;
    dialogReturnFocus = activeElement instanceof HTMLElement && activeElement !== document.body && activeElement !== document.documentElement
      ? activeElement
      : null;
  }

  function closeDialog(preserveSidebarSelection = false) {
    const returnFocus = dialogReturnFocus;
    dialogReturnFocus = null;
    requestAnimationFrame(() => {
      if (returnFocus?.isConnected) {
        if (preserveSidebarSelection) {
          returnFocus.dataset.settingsReturnFocus = "true";
        }
        returnFocus.focus();
        delete returnFocus.dataset.settingsReturnFocus;
      } else if (activeId !== null) {
        terminalFocusRequest++;
      }
    });
  }

  function modalDialogOpen(): boolean {
    return editing !== null || moving !== null || folderAction !== null || picking || importing !== null || closeConfirmation !== null || settingsOpen || broadcastConfirming || terminalPasteConfirmingSessionId !== null;
  }

  function stopLiveBroadcast(reason?: string) {
    if (liveBroadcast !== null) {
      liveBroadcastGeneration++;
    }
    const writer = liveBroadcastWriter;
    liveBroadcastWriter = null;
    liveBroadcast = null;
    liveBroadcastError = reason ?? "";
    writer?.stop(reason);
  }

  function startLiveBroadcast(snapshot: BroadcastSnapshot[]): string | null {
    liveBroadcastError = "";
    const focusedId = focusedSessionId(tabs, activeId);
    const setupError = liveBroadcastStartError(snapshot, focusedId);
    if (setupError !== null) {
      return setupError;
    }
    const validityError = liveBroadcastSnapshotInvalidationReason(snapshot, sessions, focusedId);
    if (validityError !== null) {
      return validityError;
    }

    const capturedSnapshot = snapshot.map((target) => ({ ...target }));
    liveBroadcastGeneration++;
    let writer: LiveBroadcastWriter;
    writer = createLiveBroadcastWriter(capturedSnapshot, Sessions.Write, (reason) => {
      if (liveBroadcastWriter === writer) {
        stopLiveBroadcast(reason);
      }
    });
    liveBroadcast = { snapshot: capturedSnapshot };
    liveBroadcastWriter = writer;
    liveBroadcastError = "";
    broadcastBarOpen = true;
    terminalFocusRequest++;
    return null;
  }

  function validateTerminalPaste(
    sessionId: number,
    processInstanceId: number | null,
    capturedGeneration: number,
    recipientCount: number | null,
  ): string | null {
    const generationError = liveBroadcastPasteGenerationError(capturedGeneration, liveBroadcastGeneration);
    if (generationError !== null) {
      return generationError;
    }
    if (recipientCount === null) {
      return null;
    }
    return liveBroadcastPasteError(
      liveBroadcast?.snapshot ?? null,
      sessions,
      focusedSessionId(tabs, activeId),
      sessionId,
      processInstanceId,
      recipientCount,
    );
  }

  function getLiveBroadcastPasteState(): LiveBroadcastPasteState {
    // Read parent state directly so an async paste does not depend on pane prop updates.
    return {
      generation: liveBroadcastGeneration,
      recipientCount: liveBroadcast?.snapshot.length ?? null,
    };
  }

  function handleTerminalFocusOut(sessionId: number) {
    if (liveBroadcast === null || terminalPasteConfirmingSessionId === sessionId) {
      return;
    }

    // Let tab and tile selection finish moving focus to the newly selected pane
    // before deciding whether focus left the broadcast snapshot.
    requestAnimationFrame(() => {
      const snapshot = liveBroadcast?.snapshot;
      if (snapshot === undefined) {
        return;
      }
      const activeElement = document.activeElement;
      const activePane = activeElement instanceof Element
        ? activeElement.closest<HTMLElement>("[data-terminal-session-id]")
        : null;
      const candidateSessionId = activePane === null ? null : Number(activePane.dataset.terminalSessionId);
      const nextSessionId = candidateSessionId !== null && Number.isSafeInteger(candidateSessionId)
        ? candidateSessionId
        : null;
      const invalidationReason = liveBroadcastSnapshotInvalidationReason(
        snapshot,
        sessions,
        nextSessionId,
      );
      if (invalidationReason !== null) {
        stopLiveBroadcast(invalidationReason);
      }
    });
  }

  function handleWindowBlur() {
    if (liveBroadcast !== null) {
      stopLiveBroadcast("Live broadcast stopped because the application lost focus.");
    }
  }

  function routeTerminalInput(event: TerminalInputEvent) {
    const session = sessions.find((candidate) => candidate.id === event.sessionId);
    if (
      session === undefined ||
      session.status !== "live" ||
      !isTerminalTargetCurrent(event, session.id, session.processInstanceId)
    ) {
      return;
    }
    if (liveBroadcast !== null) {
      const target = liveBroadcast.snapshot.find(
        (candidate) => candidate.logicalSessionId === event.sessionId && candidate.processInstanceId === event.processInstanceId,
      );
      if (!event.mirrorToBroadcast && target === undefined) {
        Sessions.Write(event.processInstanceId, event.input).catch(console.error);
        return;
      }
      const invalidationReason = liveBroadcastSnapshotInvalidationReason(
        liveBroadcast.snapshot,
        sessions,
        focusedSessionId(tabs, activeId),
      );
      if (invalidationReason !== null) {
        stopLiveBroadcast(invalidationReason);
        return;
      }
      if (target === undefined || liveBroadcastWriter === null) {
        stopLiveBroadcast("Live broadcast stopped: the focused terminal changed.");
        return;
      }
      if (event.mirrorToBroadcast) {
        liveBroadcastWriter.enqueue(event.input);
      } else {
        liveBroadcastWriter.enqueueTo(event.processInstanceId, event.input);
      }
      return;
    }
    Sessions.Write(event.processInstanceId, event.input).catch(console.error);
  }

  $effect(() => {
    document.documentElement.style.setProperty("--interface-scale", String(interfaceScale));
  });

  $effect(() => {
    if (liveBroadcast === null) {
      return;
    }
    const unsafeModalOpen = editing !== null || moving !== null || folderAction !== null || picking || importing !== null || scanningImport || closeConfirmation !== null || settingsOpen || broadcastConfirming;
    const reason = unsafeModalOpen
      ? "Live broadcast stopped because another action opened."
      : liveBroadcastSnapshotInvalidationReason(liveBroadcast.snapshot, sessions, focusedSessionId(tabs, activeId));
    if (reason !== null) {
      stopLiveBroadcast(reason);
    }
  });

  function persistPreference(key: PreferenceKey, value: string) {
    savePreference(localStorage, key, value);
  }

  function setThemeName(nextTheme: ThemeName) {
    themeName = nextTheme;
    document.documentElement.dataset.theme = nextTheme;
    persistPreference(preferenceKeys.themeName, nextTheme);
    updateWindowsChrome(nextTheme);
  }

  function setTerminalColors(nextColors: TerminalColors) {
    terminalColors = nextColors;
    document.documentElement.dataset.terminalColors = nextColors;
    persistPreference(preferenceKeys.terminalColors, nextColors);
  }

  function updateWindowsChrome(appearance: ThemeName) {
    if (shortcutPlatform === "windows") {
      Events.Emit("window:appearanceTheme", appearance).catch((error) =>
        console.error("Could not update the Windows title bar", error),
      );
    }
  }

  function setUiSize(nextSize: UiSize) {
    uiSize = nextSize;
    document.documentElement.dataset.uiSize = nextSize;
    persistPreference(preferenceKeys.uiSize, nextSize);
  }

  function setTerminalFontSize(nextSize: number) {
    terminalFontSize = Math.max(8, Math.min(32, nextSize));
    persistPreference(preferenceKeys.terminalFontSize, String(terminalFontSize));
  }

  function setTerminalFontName(nextFont: TerminalFontName) {
    terminalFontName = nextFont;
    persistPreference(preferenceKeys.terminalFontName, nextFont);
  }

  function openSettings() {
    if (modalDialogOpen() || scanningImport || broadcastSending) {
      return;
    }
    updatePrompt = null;
    // A completed check failure should not look like a new check when Settings reopens.
    if (!settingsOpen && !updateBusy && updateError && !updateReady) {
      updateStatus = "";
      updateError = false;
    }
    closeBroadcastBar(false);
    rememberDialogFocus();
    settingsOpen = true;
  }

  function closeSettings() {
    settingsOpen = false;
    closeDialog(true);
  }

  function toggleSidebar(focusTarget?: HTMLElement) {
    if (modalDialogOpen()) {
      return;
    }
    if (sidebarVisible && document.activeElement instanceof HTMLElement && document.activeElement.closest(".sidebar-shell")) {
      // Keep focus on a visible control when the sidebar hides.
      const requestedTarget = focusTarget?.isConnected && !focusTarget.closest(".sidebar-shell")
        ? focusTarget
        : sidebarToggleButton;
      (requestedTarget ?? document.querySelector<HTMLButtonElement>(".linux-window-header [data-menu-trigger]"))?.focus();
    }
    sidebarVisible = !sidebarVisible;
    persistPreference(preferenceKeys.sidebarVisible, String(sidebarVisible));
  }

  function toggleTilingMode() {
    if (modalDialogOpen()) {
      return;
    }
    tilingMode = !tilingMode;
    persistPreference(preferenceKeys.tilingMode, String(tilingMode));
  }

  function setRightClickToPaste(enabled: boolean) {
    rightClickToPaste = enabled;
    persistPreference(preferenceKeys.rightClickToPaste, String(rightClickToPaste));
  }

  function setCopyOnSelection(enabled: boolean) {
    copyOnSelection = enabled;
    persistPreference(preferenceKeys.copyOnSelection, String(copyOnSelection));
  }

  function toggleBroadcastBar() {
    if (modalDialogOpen() || broadcastSending || liveBroadcast !== null) {
      return;
    }
    if (broadcastBarOpen) {
      closeBroadcastBar();
    } else {
      liveBroadcastError = "";
      broadcastBarOpen = true;
    }
  }

  function closeBroadcastBar(restoreTerminalFocus = true) {
    if (liveBroadcast !== null) {
      stopLiveBroadcast("Live broadcast stopped because another action opened.");
    }
    if (!broadcastBarOpen || broadcastSending || broadcastConfirming) {
      return;
    }
    broadcastBarOpen = false;
    if (restoreTerminalFocus) {
      terminalFocusRequest++;
    }
  }

  function resizeSidebar(width: number) {
    sidebarWidth = Math.max(sidebarMinimumWidth, Math.round(width));
  }

  function finishSidebarResize(width: number) {
    persistPreference(preferenceKeys.sidebarWidth, String(Math.round(width)));
  }

  function openTab(connection: Connection | null, autofocus = true, useTiling = tilingMode) {
    openSession(connection, "ssh", autofocus, useTiling);
  }

  function openSFTP(connection: Connection) {
    openSession(connection, "sftp", true);
  }

  function focusedSession(): AppSession | null {
    const sessionId = focusedSessionId(tabs, activeId);
    return sessions.find((candidate) => candidate.id === sessionId) ?? null;
  }

  function openSFTPForFocusedSession() {
    if (modalDialogOpen()) {
      return;
    }
    const session = focusedSession();
    if (session === null) {
      return;
    }
    if (session.connection === null) {
      operationError = "SFTP needs an SSH connection; the focused session is a local shell.";
      return;
    }
    openSFTP(session.connection);
  }

  function openSession(connection: Connection | null, command: SessionCommand, autofocus: boolean, useTiling = tilingMode) {
    if (liveBroadcast !== null) {
      stopLiveBroadcast("Live broadcast stopped because another session was opened.");
    }
    if (closeConfirmation !== null) {
      return;
    }
    const placement = placeNewSession(tabs, activeId, nextSessionId, useTiling, nextTabId);
    // Joining a workspace never counts; only a new tab can exceed the limit.
    if (placement.tabs.length > maximumTabs) {
      operationError = `No more than ${maximumTabs} tabs can be open.`;
      return;
    }
    operationError = "";
    // The session keeps its own copy; editing the saved connection later does not change it.
    const session: AppSession = {
      id: nextSessionId++,
      connection: connection === null ? null : structuredClone($state.snapshot(connection)),
      command,
      status: "connecting",
      processInstanceId: null,
      autofocus,
    };
    sessions.push(session);
    tabs = placement.tabs;
    activeId = placement.activeId;
    nextTabId = placement.nextTabId;
  }

  // Removing sessions unmounts their panes, which closes their processes.
  function workspaceState(): WorkspaceState<AppSession> {
    return { tabs, sessions, activeId };
  }

  function applyWorkspaceState(state: WorkspaceState<AppSession>) {
    tabs = state.tabs;
    sessions = state.sessions;
    activeId = state.activeId;
  }

  function closeTab(id: number) {
    const next = closeTabState(workspaceState(), id);
    if (next) {
      applyWorkspaceState(next);
    }
  }

  function closeSession(sessionId: number) {
    const next = closeSessionState(workspaceState(), sessionId);
    if (next) {
      applyWorkspaceState(next);
    }
  }

  function requestCloseActiveSession() {
    if (activeTab?.kind === "session") {
      requestClose({ kind: "session", id: activeTab.sessionId });
    } else if (activeTab?.kind === "workspace") {
      requestClose({ kind: "session", id: activeTab.selectedSessionId });
    }
  }

  function handleMenuCloseTab() {
    if (settingsOpen) {
      closeSettings();
    } else if (closeConfirmation) {
      closeConfirmationDialog?.dismiss();
    } else if (importing) {
      importDialog?.dismiss();
    } else if (moving) {
      moveDialog?.dismiss();
    } else if (folderAction) {
      folderDialog?.dismiss();
    } else if (editing) {
      connectionDialog?.dismiss();
    } else if (picking) {
      quickOpenDialog?.dismiss();
    } else if (activeId !== null) {
      requestCloseActiveSession();
    }
  }

  function closeApplicationWindow() {
    WailsWindow.Close().catch((error) => console.error("Could not close the SSHBrowse window", error));
  }

  function minimiseApplicationWindow() {
    WailsWindow.Minimise().catch((error) => console.error("Could not minimise the SSHBrowse window", error));
  }

  function sessionsForCloseTarget(target: CloseTarget): AppSession[] {
    return sessionsForCloseTargetState(target, tabs, sessions);
  }

  function closeTarget(target: CloseTarget) {
    const next = closeTargetState(workspaceState(), target);
    if (next) {
      applyWorkspaceState(next);
    }
  }

  function requestClose(target: CloseTarget) {
    if (liveBroadcast !== null) {
      stopLiveBroadcast("Live broadcast stopped because a session action was opened.");
    }
    if (closeConfirmation !== null) {
      return;
    }
    const targetSessions = sessionsForCloseTarget(target);
    if (targetSessions.length === 0) {
      return;
    }
    const confirmation = closeConfirmationFor(target, tabs, sessions, displayLabels);
    if (!confirmation) {
      closeTarget(target);
      return;
    }

    rememberDialogFocus();
    closeConfirmation = confirmation;
  }

  function finishClose(confirmed: boolean) {
    const request = closeConfirmation;
    closeConfirmation = null;
    if (!request || !confirmed) {
      closeDialog();
      return;
    }

    // Resolve the target again after the dialog. A pane may have exited while
    // the confirmation was open; closed panes need no second warning.
    const currentSessions = sessionsForCloseTarget(request.target)
      .filter((session) => request.sessionIds.includes(session.id));
    if (currentSessions.length > 0) {
      closeTarget(request.target);
    }
    closeDialog();
  }

  function selectSession(sessionId: number) {
    const next = selectSessionState(tabs, sessionId);
    if (next) {
      tabs = next.tabs;
      activeId = next.activeId;
    }
  }

  function tileTab(sourceTabId: number, targetTabId: number) {
    const next = tileTabState(tabs, sourceTabId, targetTabId, nextTabId);
    if (next) {
      tabs = next.tabs;
      activeId = next.activeId;
      nextTabId = next.nextTabId;
    }
  }

  function detachSession(sessionId: number) {
    const next = detachSessionState(tabs, sessionId, nextTabId);
    if (next) {
      tabs = next.tabs;
      activeId = next.activeId;
      nextTabId = next.nextTabId;
    }
  }

  function selectIndex(index: number) {
    const next = selectTabAt(tabs, index);
    if (next !== null) {
      activeId = next;
    }
  }

  function step(delta: number) {
    const next = stepTab(tabs, activeId, delta);
    if (next !== null) {
      activeId = next;
    }
  }

  function edit(connection: Connection) {
    closeBroadcastBar(false);
    rememberDialogFocus();
    bookmarkingSessionId = null;
    editing = { connection, mode: "edit" };
  }

  function bookmark(sessionId: number) {
    const session = sessions.find((candidate) => candidate.id === sessionId);
    if (session?.connection) {
      closeBroadcastBar(false);
      rememberDialogFocus();
      bookmarkingSessionId = sessionId;
      editing = { connection: session.connection, mode: "bookmark" };
    }
  }

  function openPicker(shiftKey = false) {
    if (!modalDialogOpen() && !scanningImport) {
      closeBroadcastBar(false);
      rememberDialogFocus();
      pickerTilingMode = shiftKey ? !tilingMode : tilingMode;
      picking = true;
    }
  }

  function openPickerSelection(connection: Connection | null) {
    openTab(connection, true, pickerTilingMode);
  }

  function newConnection(folder = "") {
    if (!modalDialogOpen() && !scanningImport) {
      closeBroadcastBar(false);
      rememberDialogFocus();
      bookmarkingSessionId = null;
      editing = { connection: { ...emptyConnection(), folder }, mode: "new" };
    }
  }

  function newLocalTerminal() {
    if (!modalDialogOpen() && !scanningImport) {
      openTab(null);
    }
  }

  type TextControl = HTMLInputElement | HTMLTextAreaElement;

  function focusedTextControl(): TextControl | null {
    const active = document.activeElement;
    if (active instanceof HTMLTextAreaElement) {
      return active;
    }
    if (active instanceof HTMLInputElement && active.selectionStart !== null) {
      return active;
    }
    return null;
  }

  function replaceTextControlSelection(control: TextControl, text: string) {
    if (control.disabled || control.readOnly || document.activeElement !== control) {
      return;
    }
    const start = control.selectionStart ?? control.value.length;
    const end = control.selectionEnd ?? start;
    control.setRangeText(text, start, end, "end");
    control.dispatchEvent(new Event("input", { bubbles: true }));
  }

  function handleFormEditAction(action: EditMenuAction) {
    const control = focusedTextControl();
    if (control !== null) {
      const start = control.selectionStart ?? 0;
      const end = control.selectionEnd ?? start;
      switch (action) {
        case "copy":
          if (end > start) {
            void Clipboard.SetText(control.value.slice(start, end)).catch((err) => console.warn("Could not copy form text", err));
          }
          return;
        case "cut":
          if (end > start && !control.disabled && !control.readOnly) {
            const selected = control.value.slice(start, end);
            void Clipboard.SetText(selected)
              .then(() => replaceTextControlSelection(control, ""))
              .catch((err) => console.warn("Could not cut form text", err));
          }
          return;
        case "paste":
          void Clipboard.Text()
            .then((text) => replaceTextControlSelection(control, text))
            .catch((err) => console.warn("Could not read the clipboard for form paste", err));
          return;
        case "selectAll":
          control.select();
          return;
        case "undo":
        case "redo":
          document.execCommand(action);
          return;
      }
    }

    const active = document.activeElement;
    if (active instanceof HTMLElement && active.isContentEditable) {
      if (action === "paste") {
        void Clipboard.Text()
          .then((text) => {
            if (document.activeElement === active) {
              document.execCommand("insertText", false, text);
            }
          })
          .catch((err) => console.warn("Could not read the clipboard for editable paste", err));
        return;
      }
      if (action === "selectAll" || action === "copy" || action === "cut" || action === "undo" || action === "redo") {
        document.execCommand(action);
      }
    }
  }

  function handleEditMenuAction(event: { data: unknown }) {
    if (!isEditMenuAction(event.data)) {
      return;
    }
    const active = document.activeElement;
    if (active instanceof Element && (active.closest(".pane") !== null || active.closest("#saved-connections") !== null)) {
      return;
    }
    handleFormEditAction(event.data);
  }

  function emitEditMenuAction(action: EditMenuAction) {
    Events.Emit(editMenuEvent, action).catch((error) => console.error("Could not dispatch the Edit menu action", error));
  }

  function emitAboutMenuAction() {
    Events.Emit(aboutMenuEvent).catch((error) => console.error("Could not open the About dialog", error));
  }

  function emitCheckForUpdates() {
    startupPromptWanted = false;
    updatePrompt = null;
    if (updateReady) {
      openSettings();
      return;
    }
    Events.Emit(checkForUpdatesMenuEvent).catch((error) => {
      updateStatus = `Could not start the update check: ${errorMessage(error)}`;
      updateError = true;
      updateBusy = false;
    });
  }

  function setCheckUpdatesOnStartup(enabled: boolean) {
    checkUpdatesOnStartup = enabled;
    persistPreference(preferenceKeys.checkUpdatesOnStartup, String(enabled));
    if (!enabled) updatePrompt = null;
  }

  function openUpdateRelease() {
    if (!updateReleaseURL) return;
    Browser.OpenURL(updateReleaseURL).catch((error) => {
      updateStatus = `Could not open release notes: ${errorMessage(error)}`;
      updateError = true;
    });
  }

  function downloadFromUpdateNotice() {
    updatePrompt = null;
    openSettings();
    emitDownloadUpdate();
  }

  function emitDownloadUpdate() {
    updatePrompt = null;
    Events.Emit(updateDownloadRequestEvent).catch((error) => {
      updateStatus = `Could not start the update download: ${errorMessage(error)}`;
      updateError = true;
    });
  }

  function emitRestartUpdate() {
    Events.Emit(updateRestartRequestEvent).catch((error) => {
      updateStatus = `Could not restart to apply the update: ${errorMessage(error)}`;
      updateError = true;
      updateBusy = false;
    });
  }

  function handleApplicationShortcut(shortcut: ApplicationShortcut) {
    switch (shortcut) {
      case "newTab":
        openPicker();
        return;
      case "newLocalTerminal":
        newLocalTerminal();
        return;
      case "newSFTP":
        openSFTPForFocusedSession();
        return;
      case "newConnection":
        newConnection();
        return;
      case "closeTab":
        handleMenuCloseTab();
        return;
      case "toggleSidebar":
        toggleSidebar();
        return;
      case "settings":
        openSettings();
        return;
      case "editUndo":
        emitEditMenuAction("undo");
        return;
      case "editRedo":
        emitEditMenuAction("redo");
        return;
      case "editCut":
        emitEditMenuAction("cut");
        return;
      case "editCopy":
        emitEditMenuAction("copy");
        return;
      case "editPaste":
        emitEditMenuAction("paste");
        return;
      case "editSelectAll":
        emitEditMenuAction("selectAll");
        return;
      case "increaseFont":
        onTerminalFontCommand("increase");
        return;
      case "decreaseFont":
        onTerminalFontCommand("decrease");
        return;
      case "resetFont":
        onTerminalFontCommand("reset");
        return;
      case "minimise":
        minimiseApplicationWindow();
        return;
      case "quit":
        closeApplicationWindow();
        return;
    }
  }

  function duplicateConnection(connection: Connection, folder = connection.folder) {
    if (modalDialogOpen() || scanningImport) {
      return;
    }
    closeBroadcastBar(false);
    rememberDialogFocus();
    const duplicate = structuredClone($state.snapshot(connection));
    duplicate.id = "";
    duplicate.folder = folder;
    duplicate.provenance = null;
    bookmarkingSessionId = null;
    editing = { connection: duplicate, mode: "duplicate" };
  }

  function copyConnection(connection: Connection) {
    copiedConnection = structuredClone($state.snapshot(connection));
  }

  function requestMove(list: Connection[]) {
    if (modalDialogOpen() || scanningImport) {
      return;
    }
    closeBroadcastBar(false);
    rememberDialogFocus();
    moving = list;
  }

  async function moveToFolder(list: Connection[], folder: string) {
    await moveConnections(list, folder);
    operationError = "";
  }

  async function moveFromSidebar(list: Connection[], folder: string) {
    try {
      await moveToFolder(list, folder);
    } catch (error) {
      operationError = `Could not move connections: ${errorMessage(error)}`;
    }
  }

  async function reorderFromSidebar(ids: string[], targetId: string, after: boolean) {
    try {
      await reorderConnections(ids, targetId, after);
      operationError = "";
    } catch (error) {
      operationError = `Could not reorder connections: ${errorMessage(error)}`;
    }
  }

  function requestFolderAction(action: FolderAction) {
    if (modalDialogOpen() || scanningImport) {
      return;
    }
    closeBroadcastBar(false);
    rememberDialogFocus();
    folderAction = action;
  }

  // Moving a folder is a rename that keeps its own name under the new parent.
  async function moveFolder(path: string, newParent: string) {
    try {
      await renameFolder(path, joinFolderPath(newParent, folderBaseName(path)));
      operationError = "";
    } catch (error) {
      operationError = `Could not move folder: ${errorMessage(error)}`;
    }
  }

  // Only an empty folder can be deleted, so there is no confirmation; the
  // store's refusal for a non-empty one is shown as the error.
  async function removeFolder(path: string) {
    if (modalDialogOpen() || scanningImport) {
      return;
    }
    try {
      await deleteFolder(path);
      operationError = "";
    } catch (error) {
      operationError = `Could not delete folder: ${errorMessage(error)}`;
    }
  }

  async function removeConnections(list: Connection[]) {
    if (modalDialogOpen() || scanningImport) {
      return;
    }
    closeBroadcastBar(false);
    try {
      if (await deleteConnections(list)) {
        operationError = "";
      }
    } catch (error) {
      operationError = `Could not delete connections: ${errorMessage(error)}`;
    }
  }

  function pasteConnection(folder: string) {
    if (copiedConnection) {
      duplicateConnection(copiedConnection, folder);
    }
  }

  async function save(connection: Connection, connectAfterSave = false) {
    const saved = await saveConnection(connection);
    const session = sessions.find((candidate) => candidate.id === bookmarkingSessionId);
    if (session) {
      session.connection = saved;
    }
    if (connectAfterSave) {
      openTab(saved);
    }
  }

  function onKeydown(event: KeyboardEvent) {
    const applicationShortcut = pageApplicationShortcutFor(event);
    if (applicationShortcut !== null) {
      event.preventDefault();
      handleApplicationShortcut(applicationShortcut);
      return;
    }
    if (settingsOpen) {
      const fontCommand = terminalFontCommandFor(event, shortcutPlatform);
      if (fontCommand !== null) {
        event.preventDefault();
        onInterfaceScaleCommand(fontCommand);
      }
      return;
    }
    if (modalDialogOpen()) {
      return;
    }
    const fontCommand = terminalFontCommandFor(event, shortcutPlatform);
    if (fontCommand !== null && (activeId === null || zoomTarget === "sidebar")) {
      event.preventDefault();
      onInterfaceScaleCommand(fontCommand);
      return;
    }
    const command = tabCommandFor(event);
    if (command === null) {
      return;
    }
    event.preventDefault();
    switch (command.kind) {
      case "select":
        selectIndex(command.index);
        break;
      case "last":
        selectIndex(tabs.length - 1);
        break;
      case "next":
        step(1);
        break;
      case "previous":
        step(-1);
        break;
    }
  }

  function onTerminalFontCommand(command: TerminalFontCommand) {
    if (settingsOpen) {
      onInterfaceScaleCommand(command);
      return;
    }
    if (modalDialogOpen()) {
      return;
    }
    if (activeId === null || zoomTarget === "sidebar") {
      onInterfaceScaleCommand(command);
      return;
    }
    const detail: TerminalFontEventDetail = {
      command,
      sessionIds: liveBroadcast?.snapshot.map((target) => target.logicalSessionId) ?? [],
    };
    window.dispatchEvent(new CustomEvent<TerminalFontEventDetail>(terminalFontEvent, {
      detail,
    }));
  }

  function onInterfaceScaleCommand(command: TerminalFontCommand) {
    let nextScale = interfaceScale;
    switch (command) {
      case "increase":
        nextScale = Math.min(interfaceScaleMaximum, interfaceScale + interfaceScaleStep);
        break;
      case "decrease":
        nextScale = Math.max(interfaceScaleMinimum, interfaceScale - interfaceScaleStep);
        break;
      case "reset":
        nextScale = interfaceScaleDefault;
        break;
    }
    setInterfaceScale(nextScale);
  }

  function handleAppFocusIn(event: FocusEvent) {
    if (!(event.target instanceof HTMLElement)) {
      return;
    }
    if (event.target.closest(".sidebar-shell")) {
      zoomTarget = "sidebar";
    } else if (event.target.closest(".main")) {
      zoomTarget = "terminal";
    }
  }

  function setInterfaceScale(nextScale: number) {
    const boundedScale = Math.min(interfaceScaleMaximum, Math.max(interfaceScaleMinimum, Math.round(nextScale * 10) / 10));
    if (boundedScale === interfaceScale) {
      return;
    }
    interfaceScale = boundedScale;
    persistPreference(preferenceKeys.interfaceScale, String(boundedScale));
  }

  function emptySSHConfigScan(): SSHConfigScan {
    return { candidates: [], warnings: [], shouldPrompt: false };
  }

  async function openSSHConfigImport(firstRun = false) {
    if (modalDialogOpen() || scanningImport) {
      return;
    }
    scanningImport = true;
    try {
      const scan = await scanSSHConfig();
      if (firstRun && !scan.shouldPrompt) {
        return;
      }
      closeBroadcastBar(false);
      rememberDialogFocus();
      importing = { scan, firstRun };
    } catch (err) {
      if (!firstRun) {
        closeBroadcastBar(false);
        rememberDialogFocus();
        importing = { scan: emptySSHConfigScan(), firstRun: false, initialError: String(err) };
      } else {
        console.error(err);
      }
    } finally {
      scanningImport = false;
    }
  }

  onMount(() => {
    try {
      const preferences = loadPreferences(localStorage);
      sidebarWidth = preferences.sidebarWidth;
      sidebarVisible = preferences.sidebarVisible;
      tilingMode = preferences.tilingMode;
      terminalPasteWarningsDisabled = preferences.terminalPasteWarningsDisabled;
      rightClickToPaste = preferences.rightClickToPaste;
      copyOnSelection = preferences.copyOnSelection;
      checkUpdatesOnStartup = preferences.checkUpdatesOnStartup;
      themeName = preferences.themeName;
      terminalColors = preferences.terminalColors;
      uiSize = preferences.uiSize;
      terminalFontSize = preferences.terminalFontSize;
      terminalFontName = preferences.terminalFontName;
      document.documentElement.dataset.theme = themeName;
      document.documentElement.dataset.terminalColors = terminalColors;
      document.documentElement.dataset.uiSize = uiSize;
      interfaceScale = preferences.interfaceScale;
    } catch {
      // Preferences are optional; use the safe defaults when storage is unavailable.
    }

    updateWindowsChrome(themeName);

    loadConnections()
      .then(() => openSSHConfigImport(true))
      .catch(console.error);

    // While a dialog is up, Close Tab dismisses it and other shortcuts wait.
    const offNewTab = Events.On("menu:newTab", () => {
      openPicker();
    });
    const offNewLocalTerminal = Events.On("menu:newLocalTerminal", newLocalTerminal);
    const offNewSFTP = Events.On("menu:newSFTP", openSFTPForFocusedSession);
    const offNewConnection = Events.On("menu:newConnection", () => {
      newConnection();
    });
    const offImportSSHConfig = Events.On("menu:importSSHConfig", () => {
      openSSHConfigImport();
    });
    const offCloseTab = Events.On("menu:closeTab", handleMenuCloseTab);
    const offFontIncrease = Events.On("menu:fontIncrease", () => onTerminalFontCommand("increase"));
    const offFontDecrease = Events.On("menu:fontDecrease", () => onTerminalFontCommand("decrease"));
    const offFontReset = Events.On("menu:fontReset", () => onTerminalFontCommand("reset"));
    const offSettings = Events.On("menu:settings", openSettings);
    const offToggleSidebar = Events.On("menu:toggleSidebar", () => toggleSidebar());
    const offToggleTiling = Events.On("menu:toggleTiling", toggleTilingMode);
    const offEditMenu = Events.On(editMenuEvent, handleEditMenuAction);
    const offUpdateInfo = Events.On(updateInfoEvent, (event: { data: UpdateInfo }) => {
      updateInfo = event.data;
      if (!startupCheckScheduled && canCheckForUpdates(updateInfo.availability)) {
        startupCheckScheduled = true;
        let shouldCheck = checkUpdatesOnStartup;
        try {
          shouldCheck = claimStartupUpdateCheck(localStorage, checkUpdatesOnStartup);
        } catch {
          // A restricted WebView may deny access to localStorage itself.
        }
        if (shouldCheck) {
          // Automatic failures stay quiet; the manual check remains available.
          Events.Emit(startupUpdateCheckEvent).catch(() => {});
        }
      }
    });
    const offUpdateCheckResult = Events.On(updateCheckResultEvent, (event: { data: UpdateCheckResult }) => {
      const result = event.data;
      if (!result.automatic) {
        startupPromptWanted = false;
        updatePrompt = null;
      }
      // A late background result must not replace a manual check or download.
      if (result.automatic && (!startupPromptWanted || updateBusy || updateReady)) return;
      if (result.error) {
        if (!result.automatic) {
          updateStatus = `Update check failed: ${result.error}`;
          updateError = true;
          if (result.checked) updateBusy = false;
        }
        return;
      }
      if (!result.checked) return;
      updateStatus = result.version ? `Version ${result.version} is available.` : "SSHBrowse is up to date.";
      updateAvailable = Boolean(result.version);
      updateError = false;
      updateBusy = false;
      updateReleaseURL = "";
      if (result.version) {
        updateReleaseURL = githubReleaseURL(result.releaseURL);
        if (result.automatic && startupPromptWanted && checkUpdatesOnStartup && updateReleaseURL) {
          updatePrompt = { version: result.version, releaseURL: updateReleaseURL };
        }
      }
    });
    const offUpdateStarted = Events.On(updateCheckStartedEvent, () => {
      startupPromptWanted = false;
      updatePrompt = null;
      updateStatus = "Checking for updates…";
      updateError = false;
      updateBusy = true;
      updateAvailable = false;
      updateReleaseURL = "";
    });
    const offDownloadStarted = Events.On("wails:updater:download-started", () => {
      updatePrompt = null;
      updateStatus = "Downloading update…";
      updateError = false;
      updateBusy = true;
      updateAvailable = false;
    });
    const offDownloadProgress = Events.On("wails:updater:download-progress", (event: { data?: { written?: number; total?: number } }) => {
      const written = event.data?.written;
      const total = event.data?.total;
      if (typeof written === "number" && typeof total === "number" && total > 0) {
        updateStatus = `Downloading update… ${Math.max(0, Math.min(100, Math.floor((written / total) * 100)))}%`;
      }
    });
    const offVerifying = Events.On("wails:updater:verifying", () => {
      updateStatus = "Verifying update…";
    });
    const offInstalling = Events.On("wails:updater:installing", () => {
      updateStatus = "Installing update…";
    });
    const offUpdateReady = Events.On("wails:updater:update-ready", () => {
      updatePrompt = null;
      updateStatus = "Update ready. Restart to use the new version.";
      updateError = false;
      updateBusy = false;
      updateAvailable = false;
      updateReady = true;
    });
    const offUpdateError = Events.On("wails:updater:error", (event: { data: { stage?: string; message?: string } }) => {
      const summary = event.data?.stage === "check" ? "Update check failed" : "Update failed";
      updateStatus = event.data?.message ? `${summary}: ${event.data.message}` : `${summary}.`;
      updateError = true;
      updateBusy = false;
      updateAvailable = false;
    });
    Events.Emit(updateInfoRequestEvent).catch((error) => {
      updateStatus = `Could not read update information: ${errorMessage(error)}`;
      updateError = true;
    });
    window.addEventListener("keydown", onKeydown);
    window.addEventListener("blur", handleWindowBlur);
    return () => {
      offNewTab();
      offNewLocalTerminal();
      offNewSFTP();
      offNewConnection();
      offImportSSHConfig();
      offCloseTab();
      offFontIncrease();
      offFontDecrease();
      offFontReset();
      offSettings();
      offToggleSidebar();
      offToggleTiling();
      offEditMenu();
      offUpdateInfo();
      offUpdateCheckResult();
      offUpdateStarted();
      offDownloadStarted();
      offDownloadProgress();
      offVerifying();
      offInstalling();
      offUpdateReady();
      offUpdateError();
      window.removeEventListener("keydown", onKeydown);
      window.removeEventListener("blur", handleWindowBlur);
    };
  });
</script>

<div
  class="app"
  class:mac-window-chrome={macWindowChrome}
  class:linux-window-chrome={linuxWindowChrome}
  class:desktop-menu={desktopMenu}
  style={`--interface-scale: ${interfaceScale};`}
  onfocusin={handleAppFocusIn}
>
  {#if desktopMenu}
    <DesktopHeader
      showWindowControls={linuxWindowChrome}
      {sidebarShortcut}
      onToggleSidebar={toggleSidebar}
      onToggleTiling={toggleTilingMode}
      {newTabShortcut}
      {newLocalShortcut}
      {newSFTPShortcut}
      {newConnectionShortcut}
      {closeTabShortcut}
      {fontShortcuts}
      {minimiseShortcut}
      {quitShortcut}
      onnewtab={openPicker}
      onnewlocal={newLocalTerminal}
      onnewsftp={openSFTPForFocusedSession}
      onnewconnection={newConnection}
      onimport={() => openSSHConfigImport()}
      onclosetab={handleMenuCloseTab}
      onfont={onTerminalFontCommand}
      oneditaction={emitEditMenuAction}
      onsettings={openSettings}
      settingsActive={settingsOpen}
      onabout={emitAboutMenuAction}
      onupdate={emitCheckForUpdates}
      onclosewindow={closeApplicationWindow}
      onminimise={minimiseApplicationWindow}
    />
  {/if}
  <Sidebar
    visible={sidebarVisible}
    width={sidebarWidth}
    {interfaceScale}
    inactive={settingsOpen}
    onsettings={openSettings}
    connections={connections.list}
    folders={folders.list}
    canpaste={copiedConnection !== null}
    onopen={openTab}
    onopensftp={openSFTP}
    onedit={edit}
    onduplicate={duplicateConnection}
    ondelete={removeConnections}
    onmove={moveFromSidebar}
    onreorder={reorderFromSidebar}
    onmoverequest={requestMove}
    oncopy={copyConnection}
    onpaste={pasteConnection}
    onnew={newConnection}
    onfoldercreate={(parent) => requestFolderAction({ kind: "create", parent })}
    onfolderrename={(path) => requestFolderAction({ kind: "rename", path })}
    onfolderdelete={removeFolder}
    onfoldermove={moveFolder}
    onresize={resizeSidebar}
    onresizeend={finishSidebarResize}
  />
  <div class="main" inert={settingsOpen} aria-hidden={settingsOpen}>
    {#if linuxWindowChrome}
      <div class="linux-session-toolbar">
        <button
          bind:this={sidebarToggleButton}
          class="sidebar-toggle"
          class:active={sidebarVisible}
          aria-label={sidebarVisible ? "Hide sidebar" : "Show sidebar"}
          aria-controls="saved-connections"
          aria-expanded={sidebarVisible}
          title={`${sidebarVisible ? "Hide" : "Show"} sidebar (${sidebarShortcut})`}
          onclick={() => toggleSidebar()}
          onpointerdown={(event) => event.preventDefault()}
        >
          <PanelLeft size={16} />
        </button>
        <div class="linux-tabbar-wrap">
          <TabBar
            {tabs}
            {sessions}
            {displayLabels}
            {activeId}
            {newTabShortcut}
            {tilingMode}
            onselect={(id) => (activeId = id)}
            onclose={(id) => requestClose({ kind: "tab", id })}
            onbookmark={bookmark}
            onnew={(shiftKey) => openPicker(shiftKey)}
            ondrop={tileTab}
          />
        </div>
        <button
          class="linux-toolbar-action"
          class:active={tilingMode}
          aria-pressed={tilingMode}
          aria-label="Tiling mode"
          title={`${tilingMode ? "Disable" : "Enable"} tiling mode`}
          onclick={toggleTilingMode}
          onpointerdown={(event) => event.preventDefault()}
        >
          <LayoutGrid size={16} />
          <span>Tiling</span>
        </button>
        <button
          class="linux-toolbar-action"
          class:active={broadcastBarOpen || liveBroadcast !== null}
          aria-expanded={broadcastBarOpen}
          aria-controls="broadcast-command-bar"
          aria-label="Broadcast"
          title={`${broadcastBarOpen ? "Close" : "Open"} broadcast panel`}
          onclick={toggleBroadcastBar}
          onpointerdown={(event) => event.preventDefault()}
        >
          <Radio size={16} />
          <span>Broadcast</span>
        </button>
      </div>
    {:else}
      <div class="toolbar" class:inset={!sidebarVisible}>
      <button
        bind:this={sidebarToggleButton}
        class="sidebar-toggle"
        class:active={sidebarVisible}
        aria-label={sidebarVisible ? "Hide sidebar" : "Show sidebar"}
        aria-controls="saved-connections"
        aria-expanded={sidebarVisible}
        title={`${sidebarVisible ? "Hide" : "Show"} sidebar (${sidebarShortcut})`}
        onclick={() => toggleSidebar()}
        onpointerdown={(event) => event.preventDefault()}
      >
        <PanelLeft size={16} />
      </button>
      <TabBar
        {tabs}
        {sessions}
        {displayLabels}
        {activeId}
        {newTabShortcut}
        {tilingMode}
        onselect={(id) => (activeId = id)}
        onclose={(id) => requestClose({ kind: "tab", id })}
        onbookmark={bookmark}
        onnew={(shiftKey) => openPicker(shiftKey)}
        ondrop={tileTab}
      />
      <button
        class="toolbar-action"
        class:active={tilingMode}
        aria-pressed={tilingMode}
        aria-label="Open in tiles"
        title="Open new sessions in the active workspace, up to 9 per tab"
        onclick={toggleTilingMode}
        onpointerdown={(event) => event.preventDefault()}
      >
        <LayoutGrid size={16} />
        <span class="toolbar-label">Open in tiles</span>
        <span class="toolbar-check" aria-hidden="true"><Check size={14} /></span>
      </button>
      <button
        class="toolbar-action"
        class:active={broadcastBarOpen || liveBroadcast !== null}
        aria-expanded={broadcastBarOpen}
        aria-controls="broadcast-command-bar"
        aria-label="Broadcast"
        title={`${broadcastBarOpen ? "Close" : "Open"} broadcast panel`}
        onclick={toggleBroadcastBar}
        onpointerdown={(event) => event.preventDefault()}
      >
        <Radio size={16} />
        <span class="toolbar-label">Broadcast</span>
      </button>
      </div>
    {/if}
    {#if operationError}
      <div class="operation-error" role="alert">
        <span>{operationError}</span>
        <button aria-label="Dismiss error" title="Dismiss" onclick={() => (operationError = "")}><X size={16} /></button>
      </div>
    {/if}
    {#if broadcastBarOpen}
      <BroadcastBar
        {sessions}
        {tabs}
        {displayLabels}
        {activeId}
        enabled={!modalDialogOpen()}
        {liveBroadcast}
        broadcastError={liveBroadcastError}
        sendCommand={broadcastSender.send}
        startLiveBroadcast={startLiveBroadcast}
        stopLiveBroadcast={() => stopLiveBroadcast()}
        onclose={closeBroadcastBar}
        onsendingchange={(sending) => (broadcastSending = sending)}
        onconfirmationchange={(confirming) => (broadcastConfirming = confirming)}
      />
    {/if}
    <div
      class="panes"
      class:workspace={activeWorkspace !== null}
      style={activeTileGrid ? `--tile-columns: ${activeTileGrid.columns}; --tile-rows: ${activeTileGrid.rows};` : undefined}
    >
      {#each sessions as session (session.id)}
        {@const visible = activeTab !== null && sessionIds(activeTab).includes(session.id)}
        {@const tiled = activeWorkspace?.sessionIds.includes(session.id) ?? false}
        {@const tileIndex = activeWorkspace?.sessionIds.indexOf(session.id) ?? -1}
        {@const placement = tiled ? activeTileGrid?.placements[tileIndex] ?? null : null}
        {@const selected = activeTab?.kind === "session"
          ? activeTab.sessionId === session.id
          : activeWorkspace?.selectedSessionId === session.id}
        <div
          class="session-frame"
          class:visible
          class:tiled
          class:selected={tiled && selected}
          style:order={tiled ? activeWorkspace?.sessionIds.indexOf(session.id) : 0}
          style:grid-column={placement ? `${placement.column} / span ${placement.columnSpan}` : undefined}
          style:grid-row={placement?.row}
        >
          {#if tiled}
            <div class="tile-header">
              <button
                class="tile-select"
                onpointerdown={(event) => event.preventDefault()}
                aria-label={`Select ${sessionDisplayLabel(displayLabels, session.id)}, ${sessionStatusLabel(session.status)}`}
                onclick={() => selectSession(session.id)}
              >
                <span
                  class="tile-status"
                  class:connecting={session.status === "connecting"}
                  class:closed={session.status === "closed"}
                  aria-hidden="true"
                ></span>
                <span class="tile-name">{sessionDisplayLabel(displayLabels, session.id)}</span>
                <span
                  class="tile-state"
                  class:connecting={session.status === "connecting"}
                  class:closed={session.status === "closed"}
                >{sessionStatusLabel(session.status)}</span>
              </button>
              {#if session.connection?.id === ""}
                <button
                  aria-label={`Save ${session.connection.name}`}
                  title="Save connection"
                  onclick={(event) => {
                    event.stopPropagation();
                    bookmark(session.id);
                  }}><Bookmark size={14} /></button
                >
              {/if}
              <button
                aria-label="Move to own tab"
                onpointerdown={(event) => event.preventDefault()}
                title="Move to own tab"
                onclick={(event) => {
                  event.stopPropagation();
                  detachSession(session.id);
                }}><Ungroup size={14} /></button
              >
              <button
                aria-label={`Close ${sessionDisplayLabel(displayLabels, session.id)}`}
                onpointerdown={(event) => event.preventDefault()}
                title="Close"
                onclick={(event) => {
                  event.stopPropagation();
                  requestClose({ kind: "session", id: session.id });
                }}><X size={14} /></button
              >
            </div>
          {/if}
          {#if terminalPaneComponent}
            {@const TerminalPane = terminalPaneComponent}
            <TerminalPane
              sessionId={session.id}
              sessionLabel={sessionDisplayLabel(displayLabels, session.id)}
              connection={session.connection}
              command={session.command}
              {selected}
              autofocus={session.autofocus}
              focusRequest={terminalFocusRequest}
              shortcutsEnabled={!modalDialogOpen()}
              {rightClickToPaste}
              {copyOnSelection}
              {themeName}
              {terminalColors}
              {terminalFontName}
              defaultFontSize={terminalFontSize}
              onstatus={(status) => (session.status = status)}
              onprocesschange={(processInstanceId) => (session.processInstanceId = processInstanceId)}
              oninput={routeTerminalInput}
              onfocusout={handleTerminalFocusOut}
              onfont={onTerminalFontCommand}
              getBroadcastPasteState={getLiveBroadcastPasteState}
              pasteWarningsDisabled={terminalPasteWarningsDisabled}
              validatepaste={validateTerminalPaste}
              onpasteconfirmationchange={(confirming) => (terminalPasteConfirmingSessionId = confirming ? session.id : null)}
              onpastewarningschange={(disabled) => {
                terminalPasteWarningsDisabled = disabled;
                persistPreference(preferenceKeys.terminalPasteWarningsDisabled, String(disabled));
              }}
              onfocus={() => selectSession(session.id)}
              onclose={() => requestClose({ kind: "session", id: session.id })}
            />
          {:else if terminalPaneLoadError}
            <div class="terminal-placeholder terminal-load-error" role="alert">
              <span>{terminalPaneLoadError}</span>
              <button type="button" onclick={retryTerminalPaneLoad}>Retry</button>
            </div>
          {:else}
            <div class="terminal-placeholder" role="status">Loading terminal…</div>
          {/if}
        </div>
      {/each}
      {#if sessions.length === 0}
        <div class="empty" role="region" aria-labelledby="empty-session-title">
          <h1 id="empty-session-title">Open a session</h1>
          <p>Choose a saved connection or enter a host.</p>
          <div class="empty-actions">
            <button class="empty-action primary" onclick={() => openPicker()}>Open session</button>
            <button class="empty-action" onclick={newLocalTerminal}>Local terminal</button>
          </div>
        </div>
      {/if}
    </div>
  </div>
  {#if settingsOpen}
    <SettingsPage
      {themeName}
      {terminalColors}
      {uiSize}
      {interfaceScale}
      {terminalFontSize}
      {terminalFontName}
      {rightClickToPaste}
      {copyOnSelection}
      {sidebarWidth}
      {updateInfo}
      {updateStatus}
      {updateError}
      {updateBusy}
      {updateAvailable}
      {updateReady}
      {checkUpdatesOnStartup}
      {updateReleaseURL}
      onstartupupdatechange={setCheckUpdatesOnStartup}
      onreleasenotes={openUpdateRelease}
      oncheckforupdates={emitCheckForUpdates}
      ondownloadupdate={emitDownloadUpdate}
      onrestartupdate={emitRestartUpdate}
      onthemechange={setThemeName}
      onterminalcolorschange={setTerminalColors}
      onuisizechange={setUiSize}
      oninterfacescalechange={setInterfaceScale}
      onterminalfontsizechange={setTerminalFontSize}
      onterminalfontchange={setTerminalFontName}
      onrightclickpastechange={setRightClickToPaste}
      oncopyselectionchange={setCopyOnSelection}
      onclose={closeSettings}
    />
  {/if}
  {#if updatePrompt && updateAvailable && !updateBusy && !updateReady && !modalDialogOpen() && !scanningImport && !broadcastSending}
    <UpdateNotice
      release={updatePrompt}
      canDownload={updateInfo?.availability === "supported"}
      ondownload={downloadFromUpdateNotice}
      onreleasenotes={openUpdateRelease}
      onlater={() => { updatePrompt = null; }}
    />
  {/if}
</div>

{#if editing}
  <ConnectionForm
    {interfaceScale}
    bind:this={connectionDialog}
    connection={editing.connection}
    connections={connections.list}
    mode={editing.mode}
    folders={folderNames()}
    onsave={save}
    ondelete={deleteConnection}
    onclose={() => { editing = null; closeDialog(); }}
  />
{/if}
{#if moving}
  <MoveDialog bind:this={moveDialog} connections={moving} folders={folderNames()} onmove={moveToFolder} onclose={() => { moving = null; closeDialog(); }} />
{/if}
{#if folderAction}
  <FolderDialog bind:this={folderDialog} action={folderAction} oncreate={createFolder} onrename={renameFolder} onclose={() => { folderAction = null; closeDialog(); }} />
{/if}
{#if picking}
  <QuickOpen bind:this={quickOpenDialog} connections={connections.list} onpick={openPickerSelection} onlocal={() => openPickerSelection(null)} onclose={() => { picking = false; closeDialog(); }} />
{/if}
{#if importing}
  <SSHConfigImport
    bind:this={importDialog}
    scan={importing.scan}
    firstRun={importing.firstRun}
    initialError={importing.initialError}
    folders={folderNames()}
    onimported={loadConnections}
    onclose={() => { importing = null; closeDialog(); }}
  />
{/if}
{#if closeConfirmation}
  <CloseConfirmDialog
    bind:this={closeConfirmationDialog}
    sessionNames={closeConfirmation.sessionNames}
    workspace={closeConfirmation.workspace}
    onclose={finishClose}
  />
{/if}
<style>
  .app {
    --interface-scale: 1;
    position: relative;
    display: flex;
    height: 100vh;
    background: var(--chrome);
    color: var(--chrome-foreground);
    font: var(--ui-font-body) var(--font-ui);
  }
  .app.desktop-menu {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    grid-template-rows: calc(var(--desktop-menu-height, 46px) * var(--interface-scale)) minmax(0, 1fr);
    min-width: 640px;
  }
  .app.desktop-menu:not(.linux-window-chrome) {
    --desktop-menu-height: 34px;
  }
  .app.desktop-menu > :global(.desktop-window-header) {
    grid-column: 1 / -1;
    grid-row: 1;
  }
  .app.desktop-menu > :global(.sidebar-shell) {
    grid-column: 1;
    grid-row: 2;
  }
  .app.desktop-menu > .main {
    grid-column: 2;
    grid-row: 2;
  }
  /* Keep terminal panes outside this list so scaling never changes xterm's font or character grid. */
  .app > :global(.sidebar-shell),
  .app > :global(.desktop-window-header),
  .app .main > :global(.linux-session-toolbar),
  .app .main > :global(.toolbar),
  .app .main > :global(.operation-error),
  .app .main > :global(.broadcast-bar),
  .app .main > .panes > .empty {
    zoom: var(--interface-scale);
  }
  .app .main > .panes > .empty {
    height: calc(100% / var(--interface-scale));
  }
  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    background: var(--canvas);
  }
  .linux-session-toolbar {
    display: flex;
    flex: none;
    align-items: stretch;
    min-width: 0;
    height: 46px;
    border-bottom: 1px solid var(--toolbar-border);
    background: var(--toolbar);
  }
  .linux-tabbar-wrap {
    display: flex;
    flex: 1;
    min-width: 0;
    overflow: hidden;
  }
  .linux-tabbar-wrap :global(.tabbar) {
    width: 100%;
    height: 46px;
    padding: 10px 8px 0;
    background: transparent;
  }
  .linux-tabbar-wrap :global(.tab) {
    height: 36px;
  }
  .linux-toolbar-action {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: center;
    gap: 7px;
    height: 46px;
    min-width: 78px;
    padding: 0 14px;
    border: 0;
    border-left: 0;
    border-radius: 0;
    background: transparent;
    color: var(--toolbar-foreground);
    cursor: default;
    font: var(--ui-font-body) var(--font-ui);
  }
  .linux-toolbar-action:hover,
  .linux-toolbar-action:focus-visible,
  .linux-toolbar-action.active {
    background: var(--toolbar-control-hover);
    color: var(--toolbar-control-foreground);
  }
  .linux-toolbar-action.active {
    background: var(--toolbar-control);
    color: var(--toolbar-control-foreground);
  }
  .linux-toolbar-action:focus-visible {
    outline: 2px solid var(--focus-ring);
    outline-offset: -3px;
  }
  .toolbar {
    /* On macOS this strip also supplies the frameless drag region. */
    --wails-draggable: drag;
    container-type: inline-size;
    display: flex;
    flex: none;
    min-width: 0;
    height: 40px;
    border-bottom: 1px solid var(--toolbar-border);
    background: var(--toolbar);
  }
  /* The traffic lights sit over this strip while the sidebar is hidden on macOS. */
  .toolbar.inset {
    padding-left: 0;
  }
  :global(.app.mac-window-chrome) .toolbar.inset {
    padding-left: 72px;
  }
  .toolbar button {
    --wails-draggable: no-drag;
  }
  .operation-error {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    min-height: 34px;
    padding: 5px 10px 5px 14px;
    border-bottom: 1px solid var(--border);
    background: var(--status-error-surface);
    color: var(--text-primary);
    font: var(--ui-font-small) var(--font-ui);
  }
  .operation-error button {
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--status-error);
  }
  .sidebar-toggle {
    display: grid;
    width: 40px;
    height: 40px;
    flex: none;
    place-items: center;
    border: 0;
    border-right: 1px solid var(--toolbar-border);
    border-radius: 0;
    background: transparent;
    color: var(--toolbar-foreground);
    cursor: default;
  }
  .linux-session-toolbar .sidebar-toggle {
    height: 46px;
  }
  .sidebar-toggle:hover,
  .sidebar-toggle:focus-visible,
  .sidebar-toggle.active {
    background: var(--toolbar-control-hover);
    color: var(--toolbar-control-foreground);
  }
  .sidebar-toggle:focus-visible {
    outline: 2px solid var(--focus-ring);
    outline-offset: -3px;
  }
  .toolbar-action {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: center;
    gap: 5px;
    height: 40px;
    padding: 0 9px;
    border: 0;
    border-left: 0;
    border-radius: 0;
    background: transparent;
    color: var(--toolbar-foreground);
    cursor: default;
    font: var(--ui-font-small) var(--font-ui);
  }
  .toolbar-action:hover,
  .toolbar-action:focus-visible,
  .toolbar-action.active {
    background: var(--toolbar-control-hover);
    color: var(--toolbar-control-foreground);
  }
  .toolbar-action.active {
    background: var(--toolbar-control);
    color: var(--toolbar-control-foreground);
  }
  .toolbar-check {
    display: none;
    color: var(--toolbar-foreground);
    font-size: var(--ui-font-tiny);
    font-weight: 500;
    line-height: 1;
  }
  .toolbar-action.active .toolbar-check {
    display: inline;
  }
  .toolbar-action:focus-visible {
    outline: 2px solid var(--focus-ring);
    outline-offset: -3px;
  }
  @container (max-width: 600px) {
    .toolbar-action {
      width: 40px;
      padding: 0;
    }
    .toolbar-action .toolbar-label {
      display: none;
    }
  }
  .panes {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow: hidden;
    background: var(--canvas);
  }
  .panes.workspace {
    display: grid;
    gap: 1px;
    padding: 1px;
    background: var(--separator);
    grid-template-columns: repeat(var(--tile-columns, 1), minmax(0, 1fr));
    grid-template-rows: repeat(var(--tile-rows, 1), minmax(0, 1fr));
  }
  .session-frame {
    position: absolute;
    inset: 0;
    display: flex;
    min-width: 0;
    min-height: 0;
    flex-direction: column;
    visibility: hidden;
    background: var(--terminal-background);
    color: var(--terminal-foreground);
  }
  .terminal-placeholder {
    display: flex;
    flex: 1;
    align-items: center;
    justify-content: center;
    gap: 10px;
    padding: 24px;
    color: var(--terminal-foreground);
    text-align: center;
  }
  .terminal-load-error button {
    padding: 4px 10px;
    border: 1px solid var(--toolbar-control);
    border-radius: 4px;
    background: var(--toolbar-control);
    color: var(--toolbar-control-foreground);
    cursor: pointer;
  }
  .session-frame.visible {
    visibility: visible;
  }
  .session-frame.tiled {
    position: relative;
    inset: auto;
  }
  .session-frame.tiled.selected {
    z-index: 1;
  }
  .tile-header {
    display: flex;
    align-items: center;
    gap: 7px;
    height: 28px;
    min-height: 28px;
    padding: 0 4px 0 9px;
    border-bottom: 1px solid var(--border-subtle);
    background: var(--surface);
    color: var(--text-secondary);
    font: var(--ui-font-small) var(--font-ui);
    user-select: none;
    zoom: var(--interface-scale);
  }
  .selected .tile-header {
    background: var(--selection-inactive);
    color: var(--text-primary);
    box-shadow: inset 2px 0 var(--accent);
  }
  .session-frame.tiled.selected:focus-within .tile-header {
    box-shadow: inset 2px 0 var(--accent), inset 0 -2px var(--focus-ring);
  }
  .tile-select:focus-visible {
    outline: 2px solid var(--focus-ring);
    outline-offset: -2px;
  }
  .tile-status {
    width: 6px;
    height: 6px;
    flex: none;
    border-radius: 50%;
    background: var(--text-secondary);
  }
  .tile-status.connecting {
    border: 1px solid var(--text-secondary);
    background: transparent;
    animation: pulse 1.4s ease-in-out infinite;
  }
  .tile-status.closed {
    border: 1px solid var(--text-muted);
    border-radius: 2px;
    background: transparent;
  }
  .tile-header .tile-select {
    display: flex;
    flex: 1;
    align-items: center;
    gap: 7px;
    width: auto;
    min-width: 0;
    padding: 0 3px 0 0;
    color: inherit;
    font: inherit;
    text-align: left;
  }
  .tile-name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .tile-state {
    color: var(--text-muted);
    font-size: var(--ui-font-tiny);
  }
  .tile-header button {
    display: grid;
    place-items: center;
    width: 21px;
    height: 21px;
    padding: 0;
    border: 0;
    border-radius: 5px;
    background: none;
    color: var(--text-muted);
  }
  .tile-header > button:not(.tile-select) {
    opacity: 0;
  }
  .tile-header:hover > button:not(.tile-select),
  .tile-header:focus-within > button:not(.tile-select),
  .session-frame.tiled.selected .tile-header > button:not(.tile-select) {
    opacity: 1;
  }
  .tile-header button:hover {
    background: var(--control-hover);
    color: var(--text-primary);
  }
  .empty {
    height: 100%;
    padding: 24px;
    display: flex;
    flex-direction: column;
    gap: 12px;
    align-items: center;
    justify-content: center;
    color: var(--text-secondary);
    font: var(--ui-font-body) var(--font-ui);
    text-align: center;
  }
  .empty h1 {
    margin: 0;
    color: var(--text-primary);
    font-size: var(--ui-font-title);
    font-weight: 500;
    letter-spacing: -0.025em;
  }
  .empty p {
    margin: 0;
    color: var(--text-secondary);
    line-height: 1.5;
  }
  .empty-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 10px;
    margin-top: 10px;
  }
  .empty-action {
    min-height: 38px;
    padding: 8px 16px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--surface-raised);
    color: var(--text-primary);
    font: inherit;
    font-weight: 500;
  }
  .empty-action.primary {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--accent-foreground);
  }
  .empty-action:hover {
    border-color: var(--text-secondary);
    background: var(--surface-raised);
  }
  .empty-action.primary:hover:not(:disabled) {
    border-color: var(--accent);
    background: var(--accent-hover);
  }
</style>
