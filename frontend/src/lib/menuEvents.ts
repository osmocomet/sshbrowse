export const editMenuEvent = "menu:edit";
export const aboutMenuEvent = "menu:about";
export const checkForUpdatesMenuEvent = "menu:checkForUpdates";
export const updateInfoRequestEvent = "update:info-request";
export const updateInfoEvent = "update:info";
export const updateDownloadRequestEvent = "update:download-request";
export const updateRestartRequestEvent = "wails:updater:user:restart";

export interface UpdateInfo {
  version: string;
  availability: "development" | "package-manager" | "supported" | "unsupported" | "unavailable";
  message: string;
}
export const terminalCopyMenuEvent = "menu:terminalCopy";
export const terminalPasteMenuEvent = "menu:terminalPaste";

export type EditMenuAction = "undo" | "redo" | "cut" | "copy" | "paste" | "selectAll";

// Keep the original terminal or form target while menus are switched.
export function menuReturnFocusTarget(
  returnFocus: HTMLElement | null,
  activeMenuButton: HTMLElement | null,
): HTMLElement | null {
  return returnFocus ?? activeMenuButton;
}

export function isEditMenuAction(value: unknown): value is EditMenuAction {
  return value === "undo" || value === "redo" || value === "cut" || value === "copy" || value === "paste" || value === "selectAll";
}
