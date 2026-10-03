export interface ConnectionSelection {
  selectedIds: string[];
  anchorId: string | null;
  activeId: string | null;
}

export type SelectionMode = "replace" | "extend" | "toggle";

export function selectionModeForModifiers(shiftKey: boolean, primaryModifier: boolean): SelectionMode {
  return shiftKey ? "extend" : primaryModifier ? "toggle" : "replace";
}

export function emptyConnectionSelection(): ConnectionSelection {
  return { selectedIds: [], anchorId: null, activeId: null };
}

export function selectVisibleConnection(
  selection: ConnectionSelection,
  connectionId: string,
  visibleIds: string[],
  mode: SelectionMode,
): ConnectionSelection {
  if (mode === "extend") {
    const anchorIndex = selection.anchorId === null ? -1 : visibleIds.indexOf(selection.anchorId);
    const connectionIndex = visibleIds.indexOf(connectionId);
    if (anchorIndex >= 0 && connectionIndex >= 0) {
      const start = Math.min(anchorIndex, connectionIndex);
      const end = Math.max(anchorIndex, connectionIndex);
      return {
        selectedIds: visibleIds.slice(start, end + 1),
        anchorId: selection.anchorId,
        activeId: connectionId,
      };
    }
  }

  if (mode === "toggle") {
    if (!selection.selectedIds.includes(connectionId)) {
      return {
        selectedIds: [...selection.selectedIds, connectionId],
        anchorId: connectionId,
        activeId: connectionId,
      };
    }
    const selectedIds = selection.selectedIds.filter((id) => id !== connectionId);
    const activeId = selectedIds.at(-1) ?? null;
    return { selectedIds, anchorId: activeId, activeId };
  }

  return { selectedIds: [connectionId], anchorId: connectionId, activeId: connectionId };
}

export function selectAllVisible(selection: ConnectionSelection, visibleIds: string[]): ConnectionSelection {
  const activeId = selection.activeId && visibleIds.includes(selection.activeId)
    ? selection.activeId
    : visibleIds[0] ?? null;
  return { selectedIds: [...visibleIds], anchorId: activeId, activeId };
}

export function focusConnection(
  selection: ConnectionSelection,
  connectionId: string,
  visibleIds: string[],
): ConnectionSelection {
  if (selection.selectedIds.includes(connectionId)) {
    return { ...selection, activeId: connectionId };
  }
  return selectVisibleConnection(selection, connectionId, visibleIds, "replace");
}

// Context actions can target an unselected row without changing the user's
// selection. A selected row still becomes active so multi-row actions retain
// their existing target behavior.
export function focusContextConnection(
  selection: ConnectionSelection,
  connectionId: string,
  visibleIds: string[],
): ConnectionSelection {
  return selection.selectedIds.includes(connectionId) ? focusConnection(selection, connectionId, visibleIds) : selection;
}

function isDialogRelatedTarget(target: EventTarget | null): boolean {
  if (target === null || typeof target !== "object") {
    return false;
  }
  const closest = (target as { closest?: unknown }).closest;
  return typeof closest === "function" && closest.call(target, "dialog") !== null;
}

export function handleSidebarBackgroundFocus(
  event: Pick<FocusEvent, "target" | "currentTarget" | "relatedTarget">,
  focusingFromClick: boolean,
  contextPointerDown: boolean,
  onSelectBackground: () => void,
): void {
  if (event.target !== event.currentTarget || focusingFromClick || contextPointerDown || isDialogRelatedTarget(event.relatedTarget)) {
    return;
  }
  onSelectBackground();
}
