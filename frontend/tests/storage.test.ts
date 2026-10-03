import assert from "node:assert/strict";
import test from "node:test";
import {
  loadPreferences,
  preferenceKeys,
  saveCollapsedFolders,
  savePreference,
} from "../src/lib/storage.ts";

function storage(values: Record<string, string> = {}) {
  const entries = new Map(Object.entries(values));
  return {
    getItem(key: string) {
      return entries.get(key) ?? null;
    },
    setItem(key: string, value: string) {
      entries.set(key, value);
    },
    value(key: string) {
      return entries.get(key) ?? null;
    },
  };
}

test("current preferences load correctly", () => {
  const preferences = storage({
    [preferenceKeys.sidebarWidth]: "280",
    [preferenceKeys.sidebarVisible]: "false",
    [preferenceKeys.tilingMode]: "true",
    [preferenceKeys.rightClickToPaste]: "true",
    [preferenceKeys.copyOnSelection]: "false",
    [preferenceKeys.collapsedFolders]: JSON.stringify(["prod", 42]),
    [preferenceKeys.terminalPasteWarningsDisabled]: "true",
    [preferenceKeys.themeName]: "oled",
    [preferenceKeys.terminalColors]: "neutral",
    [preferenceKeys.uiSize]: "large",
    [preferenceKeys.terminalFontSize]: "18",
    [preferenceKeys.interfaceScale]: "1.3",
    [preferenceKeys.terminalFontName]: "jetbrains",
  });

  assert.deepEqual(loadPreferences(preferences), {
    sidebarWidth: 280,
    sidebarVisible: false,
    tilingMode: true,
    rightClickToPaste: true,
    copyOnSelection: false,
    checkUpdatesOnStartup: true,
    collapsedFolders: { prod: true },
    terminalPasteWarningsDisabled: true,
    themeName: "oled",
    terminalColors: "neutral",
    uiSize: "large",
    terminalFontSize: 18,
    interfaceScale: 1.3,
    terminalFontName: "jetbrains",
  });
});

test("missing preferences use defaults", () => {
  assert.deepEqual(loadPreferences(storage()), {
    sidebarWidth: 248,
    sidebarVisible: true,
    tilingMode: false,
    rightClickToPaste: false,
    copyOnSelection: true,
    checkUpdatesOnStartup: true,
    collapsedFolders: {},
    terminalPasteWarningsDisabled: false,
    themeName: "classic",
    terminalColors: "follow",
    uiSize: "standard",
    terminalFontSize: 14,
    interfaceScale: 1,
    terminalFontName: "system",
  });
});

test("invalid preference values fall back to safe defaults", () => {
  const preferences = storage({
    [preferenceKeys.sidebarWidth]: "not-a-width",
    [preferenceKeys.sidebarVisible]: "maybe",
    [preferenceKeys.tilingMode]: "1",
    [preferenceKeys.rightClickToPaste]: "1",
    [preferenceKeys.copyOnSelection]: "1",
    [preferenceKeys.collapsedFolders]: "{broken",
    [preferenceKeys.terminalPasteWarningsDisabled]: "maybe",
    [preferenceKeys.themeName]: "custom",
    [preferenceKeys.terminalColors]: "custom",
    [preferenceKeys.uiSize]: "huge",
    [preferenceKeys.terminalFontSize]: "500",
    [preferenceKeys.interfaceScale]: "not-a-scale",
    [preferenceKeys.terminalFontName]: "comic",
  });

  assert.deepEqual(loadPreferences(preferences), {
    sidebarWidth: 248,
    sidebarVisible: true,
    tilingMode: false,
    rightClickToPaste: false,
    copyOnSelection: true,
    checkUpdatesOnStartup: true,
    collapsedFolders: {},
    terminalPasteWarningsDisabled: false,
    themeName: "classic",
    terminalColors: "follow",
    uiSize: "standard",
    terminalFontSize: 14,
    interfaceScale: 1,
    terminalFontName: "system",
  });
});

test("older theme preferences keep following the interface", () => {
  const preferences = storage({ [preferenceKeys.themeName]: "oled" });
  assert.equal(loadPreferences(preferences).terminalColors, "follow");
  assert.equal(preferences.value(preferenceKeys.terminalColors), null);

  savePreference(preferences, preferenceKeys.terminalColors, "neutral");
  assert.equal(loadPreferences(preferences).terminalColors, "neutral");
  savePreference(preferences, preferenceKeys.terminalColors, "follow");
  assert.equal(loadPreferences(preferences).terminalColors, "follow");
});

test("explicit Warm and Moss preferences survive the new default", () => {
  assert.equal(loadPreferences(storage({ [preferenceKeys.themeName]: "warm" })).themeName, "warm");
  assert.equal(loadPreferences(storage({ [preferenceKeys.themeName]: "moss" })).themeName, "moss");
});

test("interface scale stays within supported bounds", () => {
  assert.equal(
    loadPreferences(storage({ [preferenceKeys.interfaceScale]: "2" })).interfaceScale,
    1.6,
  );
  assert.equal(
    loadPreferences(storage({ [preferenceKeys.interfaceScale]: "0.5" })).interfaceScale,
    0.8,
  );
});

test("preference writes use the current namespace", () => {
  const preferences = storage();

  savePreference(preferences, preferenceKeys.sidebarVisible, "false");
  saveCollapsedFolders(preferences, { prod: true, stale: true }, ["prod"]);

  assert.equal(preferences.value(preferenceKeys.sidebarVisible), "false");
  assert.equal(preferences.value(preferenceKeys.collapsedFolders), JSON.stringify(["prod"]));
});

test("preference reads fall back safely when storage is unavailable", () => {
  const preferences = {
    getItem() {
      throw new Error("storage unavailable");
    },
    setItem() {
      throw new Error("storage unavailable");
    },
  };

  assert.deepEqual(loadPreferences(preferences), {
    sidebarWidth: 248,
    sidebarVisible: true,
    tilingMode: false,
    rightClickToPaste: false,
    copyOnSelection: true,
    checkUpdatesOnStartup: true,
    collapsedFolders: {},
    terminalPasteWarningsDisabled: false,
    themeName: "classic",
    terminalColors: "follow",
    uiSize: "standard",
    terminalFontSize: 14,
    interfaceScale: 1,
    terminalFontName: "system",
  });
  savePreference(preferences, preferenceKeys.sidebarVisible, "false");
});
