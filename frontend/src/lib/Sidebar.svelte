<script lang="ts">
  import { onMount } from "svelte";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import Ellipsis from "@lucide/svelte/icons/ellipsis";
  import FolderPlus from "@lucide/svelte/icons/folder-plus";
  import Plus from "@lucide/svelte/icons/plus";
  import Settings from "@lucide/svelte/icons/settings";
  import { Clipboard, Events } from "@wailsio/runtime";
  import * as SidebarMenus from "../../bindings/sshbrowse/internal/app/sidebarmenus";
  import type { Connection } from "../../bindings/sshbrowse/internal/profile/models";
  import {
    buildFolderTree,
    connectionsInFolder,
    folderCreationParent,
    isWithinFolder,
    visibleConnectionIds,
    type FolderCreationContext,
    type FolderNode,
  } from "./folderTree";
  import {
    actionConnections,
    connectionAddress,
    connectionsToMove,
    folderContextMenuName,
    selectedFolderDestination,
    selectedConnections as selectedConnectionRows,
    validConnection,
    validDropFolder,
    validFolder,
    type SidebarDragPayload,
  } from "./sidebarActions";
  import {
    emptyConnectionSelection,
    focusConnection,
    focusContextConnection,
    handleSidebarBackgroundFocus,
    selectAllVisible,
    selectionModeForModifiers,
    selectVisibleConnection,
    type SelectionMode,
  } from "./sidebarSelection";
  import { currentPlatform, shortcutLabel, usesPrimaryModifier } from "./shortcuts";
  import { editMenuEvent, isEditMenuAction, type EditMenuAction } from "./menuEvents";
  import { loadPreferences, saveCollapsedFolders } from "./storage";

  let {
    connections,
    folders,
    canpaste,
    onopen,
    onopensftp,
    onedit,
    onduplicate,
    ondelete,
    onmove,
    onreorder,
    onmoverequest,
    oncopy,
    onpaste,
    onnew,
    onfoldercreate,
    onfolderrename,
    onfolderdelete,
    onfoldermove,
    visible = true,
    inactive = false,
    width = 224,
    interfaceScale = 1,
    onsettings,
    onresize,
    onresizeend,
  }: {
    connections: Connection[];
    folders: string[];
    canpaste: boolean;
    onopen: (connection: Connection, autofocus?: boolean) => void;
    onopensftp: (connection: Connection) => void;
    onedit: (connection: Connection) => void;
    onduplicate: (connection: Connection) => void;
    ondelete: (connections: Connection[]) => void;
    onmove: (connections: Connection[], folder: string) => void;
    onreorder: (ids: string[], targetId: string, after: boolean) => void;
    onmoverequest: (connections: Connection[]) => void;
    oncopy: (connection: Connection) => void;
    onpaste: (folder: string) => void;
    onnew: (folder: string) => void;
    onfoldercreate: (parent: string) => void;
    onfolderrename: (path: string) => void;
    onfolderdelete: (path: string) => void;
    onfoldermove: (path: string, newParent: string) => void;
    visible?: boolean;
    inactive?: boolean;
    width?: number;
    interfaceScale?: number;
    onsettings: () => void;
    onresize?: (width: number) => void;
    onresizeend?: (width: number) => void;
  } = $props();

  const shortcutPlatform = currentPlatform();

  function showWindowsSidebarMenu(event: MouseEvent, name: string, data: string) {
    if (shortcutPlatform !== "windows") {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    SidebarMenus.Show(name, data, event.clientX, event.clientY).catch(console.error);
  }

  let sidebar: HTMLElement;
  let collapsed = $state<Record<string, boolean>>(loadPreferences(localStorage).collapsedFolders);
  let selection = $state(emptyConnectionSelection());
  // The folder that the platform's paste shortcut and the creation buttons
  // target; "" is the top level.
  let selectedFolder = $state<string | null>("");
  // Pointer handlers manage focus so it cannot replace the selection before
  // a modified click or a context menu has chosen its targets.
  let focusingFromClick = false;
  let contextPointerDown = false;

  function focusFromClick(element: HTMLElement) {
    focusingFromClick = true;
    element.focus();
    focusingFromClick = false;
  }

  function focusSidebarBackground(event: PointerEvent) {
    if (event.button !== 0 || !(event.target instanceof HTMLElement)) {
      return;
    }
    const focusable = event.target.closest("button, [tabindex]");
    if (!focusable || focusable === sidebar) {
      sidebar.focus({ preventScroll: true });
    }
  }

  function destinationLabel(connection: Connection): string {
    return connectionAddress(connection);
  }
  let resizing = $state(false);
  let resizeStartX = 0;
  let resizeStartWidth = 224;
  let resizeCurrentWidth = 224;
  let viewportWidth = $state(typeof window === "undefined" ? 1024 : window.innerWidth);
  const minimumWidth = 180;
  const maximumWidth = 360;

  function maximumSidebarWidth(): number {
    // Keep a useful terminal area at compact window sizes.
    return Math.min(maximumWidth, Math.max(minimumWidth, viewportWidth - 420));
  }

  function clampWidth(value: number): number {
    return Math.min(maximumSidebarWidth(), Math.max(minimumWidth, Math.round(value)));
  }

  let effectiveWidth = $derived(clampWidth(width));

  function setWidth(value: number) {
    resizeCurrentWidth = clampWidth(value);
    onresize?.(resizeCurrentWidth);
  }

  function startResize(event: PointerEvent) {
    if (!visible || event.button !== 0) {
      return;
    }
    event.preventDefault();
    resizeStartX = event.clientX;
    resizeStartWidth = effectiveWidth;
    resizeCurrentWidth = effectiveWidth;
    resizing = true;
    (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  }

  function moveResize(event: PointerEvent) {
    if (!resizing) {
      return;
    }
    setWidth(resizeStartWidth + (event.clientX - resizeStartX) / interfaceScale);
  }

  function stopResize(event: PointerEvent) {
    if (!resizing) {
      return;
    }
    resizing = false;
    const target = event.currentTarget as HTMLElement;
    if (target.hasPointerCapture(event.pointerId)) {
      target.releasePointerCapture(event.pointerId);
    }
    onresizeend?.(resizeCurrentWidth);
  }

  function lostResize() {
    if (resizing) {
      resizing = false;
      onresizeend?.(resizeCurrentWidth);
    }
  }

  function resizeBy(delta: number) {
    setWidth(effectiveWidth + delta);
  }

  function resizeKeydown(event: KeyboardEvent) {
    if (event.key === "ArrowLeft") {
      event.preventDefault();
      resizeBy(-10);
    } else if (event.key === "ArrowRight") {
      event.preventDefault();
      resizeBy(10);
    } else if (event.key === "Home") {
      event.preventDefault();
      setWidth(minimumWidth);
    } else if (event.key === "End") {
      event.preventDefault();
      setWidth(maximumSidebarWidth());
    } else {
      return;
    }
    onresizeend?.(resizeCurrentWidth);
  }

  let tree = $derived(buildFolderTree(folders, connections));
  let selectedIds = $derived(selection.selectedIds.filter((id) => connections.some((connection) => connection.id === id)));
  let selectedIdSet = $derived(new Set(selectedIds));
  let activeConnection = $derived(connections.find((connection) => connection.id === selection.activeId) ?? null);
  // A renamed or deleted folder can leave a stale selection; fall back to the top level.
  let currentFolder = $derived(selectedFolderDestination(folders, selectedFolder));
  let pasteFolder = $derived(activeConnection?.folder ?? currentFolder);

  function toggle(path: string) {
    collapsed[path] = !collapsed[path];
    saveCollapsedFolders(localStorage, collapsed, folders);
  }

  function selectFolder(folder: string) {
    selection = emptyConnectionSelection();
    selectedFolder = folder;
  }

  // Plain click selects one row, the platform's primary-modifier click toggles
  // a row, and Shift-click or Shift-drag selects the visible range from the anchor.
  function selectConnection(connection: Connection, event?: Pick<MouseEvent, "shiftKey" | "metaKey" | "ctrlKey" | "altKey">) {
    const mode: SelectionMode = selectionModeForModifiers(
      event?.shiftKey ?? false,
      event !== undefined && usesPrimaryModifier(event, shortcutPlatform),
    );
    selection = selectVisibleConnection(selection, connection.id, visibleIds(), mode);
    selectedFolder = null;
  }

  function focusConnectionRow(event: FocusEvent, connection: Connection) {
    if (focusingFromClick || contextPointerDown) {
      return;
    }
    // Restoring focus after Settings must keep the current multi-selection.
    const restoringSelection = (event.currentTarget as HTMLElement).dataset.settingsReturnFocus === "true"
      || (event.relatedTarget instanceof HTMLElement && event.relatedTarget.closest("dialog") !== null);
    if (restoringSelection) {
      if (selectedIdSet.has(connection.id)) {
        selection = focusContextConnection(selection, connection.id, visibleIds());
        selectedFolder = null;
      }
      return;
    }
    selection = focusConnection(selection, connection.id, visibleIds());
    selectedFolder = null;
  }

  function focusFolderRow(event: FocusEvent, path: string) {
    if (focusingFromClick || contextPointerDown) {
      return;
    }
    const returningFromDialog = event.relatedTarget instanceof HTMLElement && event.relatedTarget.closest("dialog") !== null;
    if (!returningFromDialog) {
      selectFolder(path);
    }
  }

  function backgroundPointerDown(event: PointerEvent) {
    if (event.target !== event.currentTarget) {
      return;
    }
    if (event.button === 2 || (shortcutPlatform === "mac" && event.button === 0 && event.ctrlKey)) {
      contextPointerDown = true;
    }
  }

  function backgroundPointerUp(event: PointerEvent) {
    if (event.target !== event.currentTarget) {
      return;
    }
    if (event.button === 2 || (shortcutPlatform === "mac" && event.button === 0 && event.ctrlKey)) {
      contextPointerDown = false;
    }
  }

  function backgroundPointerCancel(event: PointerEvent) {
    if (event.target === event.currentTarget) {
      contextPointerDown = false;
    }
  }

  function visibleIds(): string[] {
    return visibleConnectionIds(tree, collapsed);
  }

  // Acting on a row that is part of the selection acts on the whole selection.
  function selectedConnections(): Connection[] {
    return selectedConnectionRows(connections, selectedIdSet);
  }

  function targetsFor(id: string): Connection[] {
    return actionConnections(connections, selectedIds, id);
  }

  function dragIdsFor(id: string): string[] {
    const targets = new Set(targetsFor(id).map((connection) => connection.id));
    return visibleConnectionIds(tree, {}).filter((connectionId) => targets.has(connectionId));
  }

  function deleteSelection() {
    const targets = selectedConnections();
    if (targets.length > 0) {
      ondelete(targets);
    } else if (currentFolder !== "" && selectedFolder !== null) {
      onfolderdelete(currentFolder);
    }
  }

  // A press becomes a drag after 5 px. Connection rows supply ordering targets;
  // folders and the list background supply move targets.
  type RangeOrigin = { x: number; y: number; pointerId: number; element: HTMLElement };
  let dragOrigin: (SidebarDragPayload & { x: number; y: number; pointerId: number; element: HTMLElement }) | null = null;
  let rangeOrigin: RangeOrigin | null = null;
  let dragging = $state(false);
  let draggingIds = $state<string[]>([]);
  let draggingFolder = $state<string | null>(null);
  let dropFolder = $state<string | null>(null);
  let dropConnection = $state<{ id: string; after: boolean } | null>(null);
  let rangeSelecting = false;
  let suppressClick = false;

  function dragStart(event: PointerEvent, payload: SidebarDragPayload) {
    suppressClick = false;
    if (event.button === 2 || (shortcutPlatform === "mac" && event.button === 0 && event.ctrlKey)) {
      contextPointerDown = true;
      return;
    }
    if (event.button !== 0) {
      return;
    }
    event.preventDefault();
    const element = event.currentTarget as HTMLElement;
    if (payload.kind === "connections" && event.shiftKey) {
      const connectionId = element.dataset.connectionId;
      if (!connectionId) {
        return;
      }
      selection = selectVisibleConnection(selection, connectionId, visibleIds(), "extend");
      selectedFolder = null;
      rangeOrigin = { x: event.clientX, y: event.clientY, pointerId: event.pointerId, element };
      element.setPointerCapture(event.pointerId);
      return;
    }
    dragOrigin = { ...payload, x: event.clientX, y: event.clientY, pointerId: event.pointerId, element };
    element.setPointerCapture(event.pointerId);
  }

  // A folder cannot be dropped into itself, its own subtree, or the folder it is already in.
  function dropTargetFor(candidate: string | undefined): string | null {
    return dragOrigin === null ? null : validDropFolder(dragOrigin, candidate);
  }

  function dragMove(event: PointerEvent) {
    if (!dragOrigin || event.pointerId !== dragOrigin.pointerId) {
      return;
    }
    if (!dragging && Math.hypot(event.clientX - dragOrigin.x, event.clientY - dragOrigin.y) < 5) {
      return;
    }
    dragging = true;
    draggingIds = dragOrigin.kind === "connections" ? dragOrigin.ids : [];
    draggingFolder = dragOrigin.kind === "folder" ? dragOrigin.path : null;
    suppressClick = true;
    const element = document.elementFromPoint(event.clientX, event.clientY);
    const draggedIds = dragOrigin.kind === "connections" ? dragOrigin.ids : [];
    const row = dragOrigin.kind === "connections" ? element?.closest<HTMLElement>("[data-connection-id]") : null;
    if (row) {
      const id = row.dataset.connectionId;
      const bounds = row.getBoundingClientRect();
      dropConnection = id && !draggedIds.includes(id)
        ? { id, after: event.clientY >= bounds.top + bounds.height / 2 }
        : null;
      dropFolder = null;
    } else {
      dropConnection = null;
      dropFolder = dropTargetFor(element?.closest<HTMLElement>("[data-folder]")?.dataset.folder);
    }
  }

  function rangeMove(event: PointerEvent) {
    if (!rangeOrigin || event.pointerId !== rangeOrigin.pointerId) {
      return;
    }
    if (!rangeSelecting && Math.hypot(event.clientX - rangeOrigin.x, event.clientY - rangeOrigin.y) < 5) {
      return;
    }
    rangeSelecting = true;
    suppressClick = true;
    const element = document.elementFromPoint(event.clientX, event.clientY)?.closest<HTMLElement>("[data-connection-id]");
    const connectionId = element?.dataset.connectionId;
    if (connectionId) {
      selection = selectVisibleConnection(selection, connectionId, visibleIds(), "extend");
      selectedFolder = null;
    }
  }

  function pointerMove(event: PointerEvent) {
    if (rangeOrigin) {
      rangeMove(event);
      return;
    }
    dragMove(event);
  }

  function drop(event: PointerEvent) {
    contextPointerDown = false;
    dragMove(event);
    const origin = dragOrigin;
    const folder = dropFolder;
    const target = dropConnection;
    const wasDragging = dragging;
    clearDrag(true);
    if (!wasDragging || origin === null) {
      return;
    }
    if (origin.kind === "connections" && target) {
      onreorder(origin.ids, target.id, target.after);
      return;
    }
    if (folder === null) {
      return;
    }
    if (origin.kind === "folder") {
      onfoldermove(origin.path, folder);
      return;
    }
    const targets = connectionsToMove(connections, origin.ids, folder);
    if (targets.length > 0) {
      onmove(targets, folder);
    }
  }

  function pointerUp(event: PointerEvent) {
    contextPointerDown = false;
    if (rangeOrigin) {
      rangeMove(event);
      clearDrag(true);
      return;
    }
    drop(event);
  }

  function clearDrag(preserveClickSuppression = false) {
    const origin = dragOrigin;
    const range = rangeOrigin;
    dragOrigin = null;
    rangeOrigin = null;
    dragging = false;
    draggingIds = [];
    draggingFolder = null;
    dropFolder = null;
    dropConnection = null;
    rangeSelecting = false;
    contextPointerDown = false;
    if (!preserveClickSuppression) {
      suppressClick = false;
    }
    if (origin?.element.hasPointerCapture(origin.pointerId)) {
      origin.element.releasePointerCapture(origin.pointerId);
    }
    if (range?.element.hasPointerCapture(range.pointerId)) {
      range.element.releasePointerCapture(range.pointerId);
    }
  }

  function copySelection(): Connection | null {
    if (!activeConnection || !selectedIdSet.has(activeConnection.id)) {
      return null;
    }
    oncopy(activeConnection);
    return activeConnection;
  }

  function copyConnectionToSystemClipboard(connection: Connection) {
    void Clipboard.SetText(connectionAddress(connection)).catch((err) => {
      console.warn("Could not copy connection to the clipboard", err);
    });
  }

  // These listeners are on the sidebar so clipboard commands elsewhere keep
  // their native WebKit behavior.
  function copy(event: ClipboardEvent) {
    const connection = copySelection();
    if (!connection) {
      return;
    }
    event.preventDefault();
    const destination = connectionAddress(connection);
    event.clipboardData?.setData("text/plain", destination);
  }

  function paste(event: ClipboardEvent) {
    if (!canpaste) {
      return;
    }
    event.preventDefault();
    onpaste(pasteFolder);
  }

  function keydown(event: KeyboardEvent) {
    const plain = !event.metaKey && !event.ctrlKey && !event.altKey && !event.shiftKey;
    const command = usesPrimaryModifier(event, shortcutPlatform) && !event.shiftKey;
    if (plain && (event.key === "Backspace" || event.key === "Delete")) {
      event.preventDefault();
      deleteSelection();
      return;
    }
    if (!command) {
      return;
    }
    const key = event.key.toLowerCase();
    if (key === "c") {
      const connection = copySelection();
      if (connection && shortcutPlatform === "linux") {
        event.preventDefault();
        // Prevent the browser's follow-up copy event so this command updates
        // the application clipboard exactly once.
        copyConnectionToSystemClipboard(connection);
      }
    } else if (key === "v" && canpaste) {
      event.preventDefault();
      onpaste(pasteFolder);
    } else if (key === "a") {
      event.preventDefault();
      selection = selectAllVisible(selection, visibleIds());
      selectedFolder = null;
    }
  }

  function handleEditMenuAction(action: EditMenuAction) {
    if (!sidebar.contains(document.activeElement)) {
      return;
    }
    switch (action) {
      case "copy": {
        const connection = copySelection();
        if (connection) {
          copyConnectionToSystemClipboard(connection);
        }
        break;
      }
      case "paste":
        if (canpaste) {
          onpaste(pasteFolder);
        }
        break;
      case "selectAll":
        selection = selectAllVisible(selection, visibleIds());
        selectedFolder = null;
        break;
      case "cut":
      case "undo":
      case "redo":
        // Saved connections have explicit copy/duplicate and delete actions;
        // a destructive cut or an edit history is not defined for the list.
        break;
    }
  }

  function actOnConnection(id: string, action: (connection: Connection) => void) {
    const connection = validConnection(connections, id);
    if (connection) {
      action(connection);
    }
  }

  function actOnFolder(path: string, action: (path: string) => void) {
    if (validFolder(folders, path)) {
      action(path);
    }
  }

  function createFolderFromContext(context: FolderCreationContext) {
    const parent = folderCreationParent(context, connections, folders);
    if (parent !== null) {
      onfoldercreate(parent);
    }
  }

  onMount(() => {
    const offOpen = Events.On("menu:connectionOpen", (event: { data: string }) => {
      const targets = targetsFor(event.data);
      targets.forEach((connection) => onopen(connection, targets.length === 1));
    });
    const offSFTP = Events.On("menu:connectionSFTP", (event: { data: string }) => targetsFor(event.data).forEach(onopensftp));
    const offEdit = Events.On("menu:connectionEdit", (event: { data: string }) => actOnConnection(event.data, onedit));
    const offDuplicate = Events.On("menu:connectionDuplicate", (event: { data: string }) => actOnConnection(event.data, onduplicate));
    const offDelete = Events.On("menu:connectionDelete", (event: { data: string }) => ondelete(targetsFor(event.data)));
    const offMove = Events.On("menu:connectionMove", (event: { data: string }) => onmoverequest(targetsFor(event.data)));
    const offFolderOpen = Events.On("menu:folderOpen", (event: { data: string }) => {
      const targets = connectionsInFolder(connections, event.data);
      targets.forEach((connection) => onopen(connection, targets.length === 1));
    });
    const offFolderNewConnection = Events.On("menu:folderNewConnection", (event: { data: string }) => actOnFolder(event.data, onnew));
    const offConnectionNewFolder = Events.On("menu:connectionNewFolder", (event: { data: string }) => {
      createFolderFromContext({ kind: "connection", id: event.data });
    });
    const offFolderCreate = Events.On("menu:folderCreate", (event: { data: string }) => {
      createFolderFromContext(event.data === "" ? { kind: "background" } : { kind: "folder", path: event.data });
    });
    const offFolderRename = Events.On("menu:folderRename", (event: { data: string }) => actOnFolder(event.data, onfolderrename));
    const offFolderDelete = Events.On("menu:folderDelete", (event: { data: string }) => actOnFolder(event.data, onfolderdelete));
    const offEditMenu = Events.On(editMenuEvent, (event: { data: unknown }) => {
      if (isEditMenuAction(event.data)) {
        handleEditMenuAction(event.data);
      }
    });
    sidebar.addEventListener("copy", copy);
    sidebar.addEventListener("paste", paste);
    sidebar.addEventListener("keydown", keydown);
    const updateViewportWidth = () => (viewportWidth = window.innerWidth);
    window.addEventListener("resize", updateViewportWidth);
    return () => {
      offOpen();
      offSFTP();
      offEdit();
      offDuplicate();
      offDelete();
      offMove();
      offConnectionNewFolder();
      offFolderOpen();
      offFolderNewConnection();
      offFolderCreate();
      offFolderRename();
      offFolderDelete();
      offEditMenu();
      sidebar.removeEventListener("copy", copy);
      sidebar.removeEventListener("paste", paste);
      sidebar.removeEventListener("keydown", keydown);
      window.removeEventListener("resize", updateViewportWidth);
    };
  });
</script>

<svelte:window onblur={() => clearDrag()} />

{#snippet connectionRow(connection: Connection, depth: number)}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    role="treeitem"
    tabindex="0"
    aria-selected={selectedIdSet.has(connection.id)}
    class="item"
    class:selected={selectedIds.includes(connection.id)}
    class:dragging={draggingIds.includes(connection.id)}
    class:drop-before={dropConnection?.id === connection.id && !dropConnection.after}
    class:drop-after={dropConnection?.id === connection.id && dropConnection.after}
    data-connection-id={connection.id}
    data-folder={connection.folder}
    style:--row-depth={depth}
    style:--custom-contextmenu-data={connection.id}
    aria-label={`${connection.name}, ${destinationLabel(connection)}`}
    title={`${connection.name} — ${destinationLabel(connection)}. Double-click to connect; drag onto a connection to reorder or a folder to move; right-click for actions`}
    onfocus={(event) => focusConnectionRow(event, connection)}
    onpointerdown={(event) => dragStart(event, { kind: "connections", ids: dragIdsFor(connection.id) })}
    onpointermove={pointerMove}
    onpointerup={pointerUp}
    onpointercancel={() => clearDrag()}
    onclick={(event) => {
      if (event.button !== 0 || (shortcutPlatform === "mac" && event.ctrlKey)) {
        return;
      }
      if (suppressClick) {
        suppressClick = false;
        return;
      }
      focusFromClick(event.currentTarget);
      selectConnection(connection, event);
    }}
    oncontextmenu={(event) => {
      contextPointerDown = false;
      focusFromClick(event.currentTarget);
      selection = focusContextConnection(selection, connection.id, visibleIds());
      showWindowsSidebarMenu(event, "saved-connection", connection.id);
    }}
    ondblclick={() => onopen(connection, false)}
    onkeydown={(event) => {
      if (event.target !== event.currentTarget) {
        return;
      }
      if (event.key === "Enter") {
        event.preventDefault();
        const targets = targetsFor(connection.id);
        targets.forEach((target) => onopen(target, targets.length === 1));
      } else if (event.key === " ") {
        event.preventDefault();
        selectConnection(connection, event);
      }
    }}
  >
    <span class="copy">
      <span class="name">{connection.name}</span>
      <span class="detail">{destinationLabel(connection)}</span>
    </span>
    <button
      type="button"
      class="more"
      tabindex="0"
      aria-haspopup="menu"
      aria-label={`Actions for ${connection.name}`}
      title="Connection actions"
      onclick={(event) => {
        event.stopPropagation();
        const trigger = event.currentTarget as HTMLElement;
        const bounds = trigger.getBoundingClientRect();
        trigger.dispatchEvent(new MouseEvent("contextmenu", {
          bubbles: true,
          cancelable: true,
          button: 2,
          clientX: bounds.left + bounds.width / 2,
          clientY: bounds.top + bounds.height / 2,
        }));
      }}
      onpointerdown={(event) => event.stopPropagation()}
      ondblclick={(event) => event.stopPropagation()}
    >
      <Ellipsis size={16} />
    </button>
  </div>
{/snippet}

{#snippet folderRow(folder: FolderNode<Connection>, depth: number)}
  <div class="folder-block" role="group">
    <button
      type="button"
      role="treeitem"
      aria-selected={currentFolder === folder.path && selectedFolder !== null && selectedIds.length === 0}
      class="group"
      class:selected={currentFolder === folder.path && selectedFolder !== null && selectedIds.length === 0}
      class:dragging={draggingFolder !== null && isWithinFolder(folder.path, draggingFolder)}
      class:drop-target={dragging && dropFolder === folder.path}
      data-folder={folder.path}
      style:--row-depth={depth}
      style:--custom-contextmenu={folderContextMenuName(connections, folder.path)}
      style:--custom-contextmenu-data={folder.path}
      aria-label={folder.path}
      aria-expanded={!collapsed[folder.path]}
      title="Click to collapse or expand; drag onto a folder to move; right-click for actions"
      onfocus={(event) => focusFolderRow(event, folder.path)}
      onpointerdown={(event) => dragStart(event, { kind: "folder", path: folder.path })}
      onpointermove={dragMove}
      onpointerup={drop}
      onpointercancel={() => clearDrag()}
      onclick={(event) => {
        if (suppressClick) {
          suppressClick = false;
          return;
        }
        event.currentTarget.focus();
        selectFolder(folder.path);
        toggle(folder.path);
      }}
      oncontextmenu={(event) => {
        contextPointerDown = false;
        focusFromClick(event.currentTarget);
        showWindowsSidebarMenu(event, folderContextMenuName(connections, folder.path), folder.path);
      }}
    >
      <span class="chevron" aria-hidden="true">
        {#if collapsed[folder.path]}<ChevronRight size={14} />{:else}<ChevronDown size={14} />{/if}
      </span>
      <span class="folder-name">{folder.name}</span>
    </button>
    {#if !collapsed[folder.path]}
      {#each folder.connections as connection (connection.id)}
        {@render connectionRow(connection, depth + 1)}
      {/each}
      {#each folder.children as child (child.path)}
        {@render folderRow(child, depth + 1)}
      {/each}
    {/if}
  </div>
{/snippet}

<div
  class="sidebar-shell"
  class:is-hidden={!visible}
  class:is-resizing={resizing}
  inert={!visible || inactive}
  style={`width: ${visible ? effectiveWidth : 0}px`}
>
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<aside bind:this={sidebar} id="saved-connections" class="sidebar" tabindex="-1" aria-label="Saved connections" aria-hidden={!visible || inactive} inert={!visible || inactive} onpointerdown={focusSidebarBackground}>
  <div class="header">
    <span class="header-actions">
      <button class="icon folder-add" aria-label="New folder" title="New folder" onclick={() => onfoldercreate(pasteFolder)}>
        <FolderPlus size={18} />
      </button>
      <button type="button" class="icon connection-add" aria-label="New connection" title={`New connection (${shortcutLabel("⌘N", "Ctrl+Alt+N", shortcutPlatform)})`} onclick={() => onnew(currentFolder)}>
        <Plus size={18} />
      </button>
    </span>
  </div>

  <!-- The list background is the top-level drop and paste target, so no row is drawn for it. -->
  <!-- Row elements override this inherited menu. Context events must keep
       bubbling to Wails' window listener so row menus remain native. -->
  <div
    class="list"
    class:root-drop={dragging && dropFolder === ""}
    data-folder=""
    style:--custom-contextmenu="saved-sidebar"
    style:--custom-contextmenu-data=""
    tabindex="0"
    role="tree"
    aria-label="Saved connections"
    onfocus={(event) => handleSidebarBackgroundFocus(event, focusingFromClick, contextPointerDown, () => selectFolder(""))}
    onpointerdown={backgroundPointerDown}
    onpointerup={backgroundPointerUp}
    onpointercancel={backgroundPointerCancel}
    oncontextmenu={(event) => {
      if (event.target !== event.currentTarget) {
        return;
      }
      contextPointerDown = false;
      focusFromClick(event.currentTarget);
      showWindowsSidebarMenu(event, "saved-sidebar", "");
    }}
    onclick={(event) => event.target === event.currentTarget && selectFolder("")}
    onkeydown={(event) => event.key === "Enter" && event.target === event.currentTarget && selectFolder("")}
  >
    {#each tree.connections as connection (connection.id)}
      {@render connectionRow(connection, 0)}
    {/each}
    {#each tree.folders as folder (folder.path)}
      {@render folderRow(folder, 0)}
    {/each}
  </div>
  <div class="sidebar-footer">
    <button type="button" class="settings-button" onclick={onsettings} title="Settings">
      <Settings size={18} />
      <span>Settings</span>
    </button>
  </div>
</aside>
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="resize-handle"
  role="separator"
  aria-label="Resize sidebar"
  aria-orientation="vertical"
  aria-hidden={!visible}
  aria-valuemin={minimumWidth}
  aria-valuemax={maximumSidebarWidth()}
  aria-valuenow={effectiveWidth}
  tabindex={visible ? 0 : -1}
  onpointerdown={startResize}
  onpointermove={moveResize}
  onpointerup={stopResize}
  onpointercancel={stopResize}
  onlostpointercapture={lostResize}
  onkeydown={resizeKeydown}
></div>
</div>

<style>
  .sidebar {
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    border-right: 1px solid var(--sidebar-border);
    background: var(--sidebar);
    color: var(--sidebar-foreground);
    font: var(--ui-font-body) var(--font-ui);
    user-select: none;
  }
  .sidebar-shell:focus-within .sidebar {
    box-shadow: inset 0 0 0 1px var(--sidebar-border);
  }
  .sidebar:focus {
    outline: none;
  }
  .sidebar-shell {
    position: relative;
    /* The parent grid or flex row already provides the height needed at this element's zoom. */
    height: 100%;
    flex: none;
    min-width: 0;
    transition: width 160ms ease;
  }
  .sidebar-shell.is-hidden {
    overflow: hidden;
    pointer-events: none;
  }
  .sidebar-shell.is-resizing {
    transition: none;
    cursor: col-resize;
  }
  @media (prefers-reduced-motion: reduce) {
    .sidebar-shell {
      transition: none;
    }
  }
  .resize-handle {
    position: absolute;
    top: 0;
    right: -4px;
    bottom: 0;
    z-index: 2;
    width: 8px;
    cursor: col-resize;
    touch-action: none;
  }
  .resize-handle::after {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 3px;
    width: 1px;
    background: transparent;
    content: "";
    transition: background 120ms ease;
  }
  .resize-handle:hover::after,
  .is-resizing .resize-handle::after {
    background: var(--accent);
  }
  .resize-handle:focus-visible::after {
    background: var(--focus-ring);
  }
  .resize-handle:focus-visible {
    outline: none;
  }
  .header {
    /* On macOS the hidden title bar leaves the traffic lights over this strip. */
    --wails-draggable: drag;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    height: 40px;
    padding: 0 10px;
    border-bottom: 1px solid var(--sidebar-border);
  }
  :global(.app.mac-window-chrome) .header {
    padding-left: 72px;
  }
  :global(.app.linux-window-chrome) .header {
    --wails-draggable: no-drag;
    height: 46px;
    padding: 0 12px;
  }
  :global(.app.linux-window-chrome) .header-actions {
    gap: 4px;
  }
  .header button {
    --wails-draggable: no-drag;
  }
  .header-actions {
    display: flex;
    align-items: center;
    gap: 2px;
  }
  .list {
    flex: 1;
    overflow-y: auto;
    padding: 8px 8px 12px;
  }
  .sidebar-footer { flex: none; padding: 8px; border-top: 1px solid var(--sidebar-border); }
  .settings-button {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    min-height: 38px;
    padding: 7px 10px;
    border: 0;
    border-radius: var(--radius-control);
    background: transparent;
    color: var(--icon-muted);
    font: inherit;
    text-align: left;
    cursor: default;
  }
  .settings-button:hover { background: var(--sidebar-control-surface); color: var(--sidebar-foreground); }
  .list:focus {
    outline: none;
  }
  .list:focus-visible {
    outline: none;
    box-shadow: inset 2px 0 var(--focus-ring);
  }
  .list.root-drop {
    box-shadow: inset 0 0 0 2px var(--accent);
  }
  .group {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    border: 1px solid transparent;
    background: none;
    color: var(--sidebar-foreground);
    font: var(--ui-font-body) var(--font-ui);
    font-weight: 500;
    min-height: 32px;
    padding: 5px 8px 5px calc(8px + var(--row-depth) * 14px);
    border-radius: var(--radius-control);
    text-align: left;
    cursor: default;
  }
  .chevron {
    display: grid;
    flex: none;
    width: 14px;
    place-items: center;
  }
  .folder-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .item {
    --custom-contextmenu: saved-connection;
    display: flex;
    align-items: center;
    width: 100%;
    min-height: 40px;
    padding: 4px 5px 4px calc(8px + var(--row-depth) * 14px);
    border: 1px solid transparent;
    border-radius: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: default;
  }
  .item:hover,
  .group:hover {
    background: var(--sidebar-row-hover);
    color: var(--sidebar-foreground);
  }
  .item.selected,
  .group.selected {
    background: var(--sidebar-row-selected);
    color: var(--sidebar-foreground);
    outline: none;
  }
  .sidebar:focus-within .item.selected,
  .sidebar:focus-within .group.selected {
    background: var(--sidebar-row-active);
  }
  .item.dragging,
  .group.dragging {
    opacity: 0.5;
  }
  .group.drop-target {
    outline: 2px solid var(--accent);
    outline-offset: -2px;
  }
  .item.drop-before {
    box-shadow: inset 0 2px var(--accent);
  }
  .item.drop-after {
    box-shadow: inset 0 -2px var(--accent);
  }
  .item:focus-visible,
  .group:focus-visible {
    outline: none;
    box-shadow: inset 2px 0 var(--focus-ring);
  }
  .item.drop-before:focus-visible {
    box-shadow: inset 2px 0 var(--focus-ring), inset 0 2px var(--accent);
  }
  .item.drop-after:focus-visible {
    box-shadow: inset 2px 0 var(--focus-ring), inset 0 -2px var(--accent);
  }
  .copy {
    min-width: 0;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .more {
    display: grid;
    place-items: center;
    flex: none;
    width: 28px;
    height: 28px;
    padding: 0;
    border: 0;
    border-radius: 6px;
    background: none;
    color: var(--icon-muted);
    opacity: 0;
  }
  .item:hover .more,
  .item:focus-within .more,
  .more:focus-visible {
    opacity: 1;
  }
  .more:hover {
    background: var(--sidebar-control-surface);
    color: var(--sidebar-foreground);
  }
  .more:focus-visible {
    outline: 2px solid var(--focus-ring);
    outline-offset: -2px;
  }
  .name {
    overflow: hidden;
    color: var(--sidebar-foreground);
    font-weight: 500;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .detail {
    overflow: hidden;
    color: var(--sidebar-muted-foreground);
    font-size: var(--ui-font-small);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .icon {
    display: grid;
    place-items: center;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    width: 30px;
    height: 30px;
    border-radius: 4px;
    font-size: 15px;
    line-height: 1;
    cursor: default;
  }
  .icon:hover {
    background: var(--sidebar-control-surface);
    color: var(--sidebar-foreground);
  }
</style>
