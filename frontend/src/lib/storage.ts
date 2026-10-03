import type { TerminalColors, TerminalFontName, ThemeName } from "./appearance";

export type PreferenceStorage = Pick<Storage, "getItem" | "setItem">;

export type UiSize = "standard" | "large";

export const preferenceKeys = {
  sidebarWidth: "sshbrowse.sidebar.width",
  sidebarVisible: "sshbrowse.sidebar.visible",
  tilingMode: "sshbrowse.tiling.enabled",
  rightClickToPaste: "sshbrowse.terminal.rightClickToPaste",
  copyOnSelection: "sshbrowse.terminal.copyOnSelection",
  collapsedFolders: "sshbrowse.sidebar.collapsed",
  terminalPasteWarningsDisabled: "sshbrowse.terminal.pasteWarningsDisabled",
  themeName: "sshbrowse.appearance.theme",
  terminalColors: "sshbrowse.appearance.terminalColors",
  uiSize: "sshbrowse.appearance.uiSize",
  terminalFontSize: "sshbrowse.appearance.terminalFontSize",
  interfaceScale: "sshbrowse.interface.scale",
  terminalFontName: "sshbrowse.appearance.terminalFontName",
} as const;
export type PreferenceKey = typeof preferenceKeys[keyof typeof preferenceKeys];

export const sidebarMinimumWidth = 180;
export const sidebarMaximumWidth = 360;
export const sidebarDefaultWidth = 248;
export const interfaceScaleMinimum = 0.8;
export const interfaceScaleMaximum = 1.6;
export const interfaceScaleDefault = 1;
export const interfaceScaleStep = 0.1;

export interface Preferences {
  sidebarWidth: number;
  sidebarVisible: boolean;
  tilingMode: boolean;
  rightClickToPaste: boolean;
  copyOnSelection: boolean;
  collapsedFolders: Record<string, boolean>;
  terminalPasteWarningsDisabled: boolean;
  themeName: ThemeName;
  terminalColors: TerminalColors;
  uiSize: UiSize;
  terminalFontSize: number;
  interfaceScale: number;
  terminalFontName: TerminalFontName;
}

function readValue(storage: PreferenceStorage, key: string): string | null {
  try {
    return storage.getItem(key);
  } catch {
    return null;
  }
}

function readBoolean(storage: PreferenceStorage, key: string, fallback: boolean): boolean {
  const value = readValue(storage, key);
  return value === "true" ? true : value === "false" ? false : fallback;
}

function readSidebarWidth(storage: PreferenceStorage): number {
  const value = Number(readValue(storage, preferenceKeys.sidebarWidth));
  if (!Number.isFinite(value) || value < sidebarMinimumWidth) {
    return sidebarDefaultWidth;
  }
  return Math.min(sidebarMaximumWidth, Math.round(value));
}

function readInterfaceScale(storage: PreferenceStorage): number {
  const raw = readValue(storage, preferenceKeys.interfaceScale);
  if (raw === null) {
    return interfaceScaleDefault;
  }
  const value = Number(raw);
  if (!Number.isFinite(value)) {
    return interfaceScaleDefault;
  }
  return Math.min(interfaceScaleMaximum, Math.max(interfaceScaleMinimum, Math.round(value * 10) / 10));
}

function readCollapsedFolders(storage: PreferenceStorage): Record<string, boolean> {
  const raw = readValue(storage, preferenceKeys.collapsedFolders);
  if (raw === null) {
    return {};
  }
  try {
    const stored: unknown = JSON.parse(raw);
    if (!Array.isArray(stored)) {
      return {};
    }
    return Object.fromEntries(
      stored.filter((path): path is string => typeof path === "string").map((path) => [path, true]),
    );
  } catch {
    return {};
  }
}

function readThemeName(storage: PreferenceStorage): ThemeName {
  const value = readValue(storage, preferenceKeys.themeName);
  return value === "warm" || value === "moss" || value === "fjord" || value === "oled" || value === "contrast" ? value : "classic";
}

function readTerminalColors(storage: PreferenceStorage): TerminalColors {
  // Older profiles followed the interface palette; absence preserves that behavior.
  return readValue(storage, preferenceKeys.terminalColors) === "neutral" ? "neutral" : "follow";
}

function readTerminalFontName(storage: PreferenceStorage): TerminalFontName {
  const value = readValue(storage, preferenceKeys.terminalFontName);
  return value === "jetbrains" || value === "menlo" || value === "consolas" || value === "dejavu" ? value : "system";
}

function readUiSize(storage: PreferenceStorage): UiSize {
  return readValue(storage, preferenceKeys.uiSize) === "large" ? "large" : "standard";
}

function readTerminalFontSize(storage: PreferenceStorage): number {
  const value = Number(readValue(storage, preferenceKeys.terminalFontSize));
  return Number.isInteger(value) && value >= 8 && value <= 32 ? value : 14;
}

export function loadPreferences(storage: PreferenceStorage): Preferences {
  return {
    sidebarWidth: readSidebarWidth(storage),
    sidebarVisible: readBoolean(storage, preferenceKeys.sidebarVisible, true),
    tilingMode: readBoolean(storage, preferenceKeys.tilingMode, false),
    rightClickToPaste: readBoolean(storage, preferenceKeys.rightClickToPaste, false),
    copyOnSelection: readBoolean(storage, preferenceKeys.copyOnSelection, true),
    collapsedFolders: readCollapsedFolders(storage),
    terminalPasteWarningsDisabled: readBoolean(storage, preferenceKeys.terminalPasteWarningsDisabled, false),
    themeName: readThemeName(storage),
    terminalColors: readTerminalColors(storage),
    uiSize: readUiSize(storage),
    terminalFontSize: readTerminalFontSize(storage),
    interfaceScale: readInterfaceScale(storage),
    terminalFontName: readTerminalFontName(storage),
  };
}

export function savePreference(storage: PreferenceStorage, key: PreferenceKey, value: string): void {
  try {
    storage.setItem(key, value);
  } catch {
    // Preferences are optional; private browsing and restricted storage are valid states.
  }
}

export function saveCollapsedFolders(
  storage: PreferenceStorage,
  collapsed: Record<string, boolean>,
  folders: readonly string[],
): void {
  savePreference(
    storage,
    preferenceKeys.collapsedFolders,
    JSON.stringify(folders.filter((folder) => collapsed[folder])),
  );
}
