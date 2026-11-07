// Tab switching and terminal-safe application shortcuts are handled in the page.
export type ShortcutPlatform = "mac" | "linux" | "windows";

export function currentPlatform(): ShortcutPlatform {
  if (typeof navigator === "undefined") {
    // Keep non-browser callers deterministic.
    return "mac";
  }
  const platform = `${navigator.platform} ${navigator.userAgent}`.toLowerCase();
  if (platform.includes("win")) {
    return "windows";
  }
  return platform.includes("mac") ? "mac" : "linux";
}

export function usesPrimaryModifier(
  event: Pick<KeyboardEvent, "metaKey" | "ctrlKey" | "altKey">,
  platform: ShortcutPlatform = currentPlatform(),
): boolean {
  if (event.altKey) {
    return false;
  }
  return platform === "mac"
    ? event.metaKey && !event.ctrlKey
    : event.ctrlKey && !event.metaKey;
}

export function shortcutLabel(mac: string, linux: string, platform: ShortcutPlatform = currentPlatform()): string {
  return platform === "mac" ? mac : linux;
}

export type TabCommand =
  | { kind: "select"; index: number } // Primary+1..8; Primary+9 is the last tab, like browsers
  | { kind: "last" }
  | { kind: "next" } // Ctrl+Tab or Primary+Shift+]
  | { kind: "previous" }; // Ctrl+Shift+Tab or Primary+Shift+[

export type TerminalFontCommand = "increase" | "decrease" | "reset";

export interface TerminalFontEventDetail {
  command: TerminalFontCommand;
  sessionIds: readonly number[];
}

export type ApplicationShortcut =
  | "newTab"
  | "newLocalTerminal"
  | "newSFTP"
  | "newConnection"
  | "closeTab"
  | "toggleSidebar"
  | "settings"
  | "editUndo"
  | "editRedo"
  | "editCut"
  | "editCopy"
  | "editPaste"
  | "editSelectAll"
  | "increaseFont"
  | "decreaseFont"
  | "resetFont"
  | "minimise"
  | "quit";

export const terminalFontEvent = "sshbrowse:terminal-font";

export function terminalSearchShortcutFor(
  event: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">,
  platform: ShortcutPlatform = currentPlatform(),
): boolean {
  if (!usesPrimaryModifier(event, platform) || event.key.toLowerCase() !== "f") {
    return false;
  }
  // Ctrl+F remains available to shells and terminal applications.
  return platform === "mac" ? !event.shiftKey : event.shiftKey;
}

export function terminalFontCommandFor(
  event: KeyboardEvent,
  platform: ShortcutPlatform = currentPlatform(),
): TerminalFontCommand | null {
  if (!usesPrimaryModifier(event, platform)) {
    return null;
  }
  if (event.key === "=" || event.key === "+") {
    return "increase";
  }
  if (event.key === "-") {
    return "decrease";
  }
  if (event.key === "0") {
    return "reset";
  }
  return null;
}

function applicationShortcutFor(
  event: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">,
  platform: "linux" | "windows",
): ApplicationShortcut | null {
  if (!event.ctrlKey || event.metaKey) {
    return null;
  }
  const key = event.key.toLowerCase();
  if (event.altKey) {
    if (key === "z") {
      return event.shiftKey ? "editRedo" : "editUndo";
    }
    if (!event.shiftKey && key === "n") {
      return "newConnection";
    }
    if (!event.shiftKey && key === "m") {
      return "minimise";
    }
    if (!event.shiftKey && key === "q") {
      return "quit";
    }
    return null;
  }
  if (event.shiftKey) {
    switch (key) {
      case "n":
        return "newTab";
      case "t":
        return "newLocalTerminal";
      case "p":
        return "newSFTP";
      case "w":
        return "closeTab";
      case "b":
        return "toggleSidebar";
      case "c":
        return "editCopy";
      case "v":
        return "editPaste";
      case "x":
        return "editCut";
      case "a":
        return "editSelectAll";
      case "+":
      case "=":
        return "increaseFont";
      default:
        return null;
    }
  }
  switch (key) {
    case ",":
      return "settings";
    case "+":
    case "=":
      return "increaseFont";
    case "-":
      return "decreaseFont";
    case "0":
      return "resetFont";
    default:
      return null;
  }
}

export function tabCommandFor(
  event: KeyboardEvent,
  platform: ShortcutPlatform = currentPlatform(),
): TabCommand | null {
  // Control cycles tabs on every platform, including macOS.
  if (event.key === "Tab" && event.ctrlKey && !event.metaKey && !event.altKey) {
    return { kind: event.shiftKey ? "previous" : "next" };
  }
  if (!usesPrimaryModifier(event, platform)) {
    return null;
  }
  if (!event.shiftKey && event.key >= "1" && event.key <= "8") {
    return { kind: "select", index: Number(event.key) - 1 };
  }
  if (!event.shiftKey && event.key === "9") {
    return { kind: "last" };
  }
  if (event.shiftKey && event.code === "BracketRight") {
    return { kind: "next" };
  }
  if (event.shiftKey && event.code === "BracketLeft") {
    return { kind: "previous" };
  }
  return null;
}

export function linuxShortcutFor(
  event: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">,
  platform: ShortcutPlatform = currentPlatform(),
): ApplicationShortcut | null {
  return platform === "linux" ? applicationShortcutFor(event, platform) : null;
}

export function windowsShortcutFor(
  event: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">,
  platform: ShortcutPlatform = currentPlatform(),
): ApplicationShortcut | null {
  return platform === "windows" ? applicationShortcutFor(event, platform) : null;
}

// The frontend menu owns application shortcuts on Linux and Windows.
export function pageApplicationShortcutFor(
  event: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">,
  platform: ShortcutPlatform = currentPlatform(),
): ApplicationShortcut | null {
  return platform === "windows" ? windowsShortcutFor(event, platform) : linuxShortcutFor(event, platform);
}
