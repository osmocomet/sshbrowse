import assert from "node:assert/strict";
import test from "node:test";

import {
  linuxShortcutFor,
  pageApplicationShortcutFor,
  shortcutLabel,
  tabCommandFor,
  terminalFontCommandFor,
  usesPrimaryModifier,
  windowsShortcutFor,
} from "../src/lib/shortcuts.ts";
import { macTerminalEditingInputFor } from "../src/lib/terminalInput.ts";

function keyboardEvent(overrides: Partial<KeyboardEvent> = {}): KeyboardEvent {
  return {
    type: "keydown",
    key: "",
    code: "",
    shiftKey: false,
    ctrlKey: false,
    altKey: false,
    metaKey: false,
    ...overrides,
  } as KeyboardEvent;
}

test("Linux uses Ctrl for page shortcuts and rejects the macOS modifier", () => {
  assert.deepEqual(tabCommandFor(keyboardEvent({ key: "1", ctrlKey: true }), "linux"), { kind: "select", index: 0 });
  assert.deepEqual(
    tabCommandFor(keyboardEvent({ code: "BracketRight", key: "]", ctrlKey: true, shiftKey: true }), "linux"),
    { kind: "next" },
  );
  assert.equal(tabCommandFor(keyboardEvent({ key: "1", metaKey: true }), "linux"), null);
  assert.equal(tabCommandFor(keyboardEvent({ key: "1", ctrlKey: true, metaKey: true }), "linux"), null);
});

test("macOS keeps Command tab shortcuts and does not claim Ctrl input", () => {
  assert.deepEqual(tabCommandFor(keyboardEvent({ key: "9", metaKey: true }), "mac"), { kind: "last" });
  assert.equal(tabCommandFor(keyboardEvent({ key: "9", ctrlKey: true }), "mac"), null);
});

test("Windows uses Ctrl for terminal-safe tab and font shortcuts", () => {
  assert.deepEqual(tabCommandFor(keyboardEvent({ key: "1", ctrlKey: true }), "windows"), { kind: "select", index: 0 });
  assert.equal(terminalFontCommandFor(keyboardEvent({ key: "=", ctrlKey: true }), "windows"), "increase");
  assert.equal(linuxShortcutFor(keyboardEvent({ key: "t", ctrlKey: true, shiftKey: true }), "windows"), null);
});

test("Windows application shortcuts cover terminal-safe actions and editing", () => {
  const shifted = new Map([
    ["n", "newTab"],
    ["t", "newLocalTerminal"],
    ["p", "newSFTP"],
    ["w", "closeTab"],
    ["b", "toggleSidebar"],
    ["c", "editCopy"],
    ["v", "editPaste"],
    ["x", "editCut"],
    ["a", "editSelectAll"],
  ]);
  for (const [key, shortcut] of shifted) {
    assert.equal(windowsShortcutFor(keyboardEvent({ key, ctrlKey: true, shiftKey: true }), "windows"), shortcut);
  }
  for (const [key, shortcut] of new Map([
    ["n", "newConnection"],
    ["m", "minimise"],
    ["q", "quit"],
    ["z", "editUndo"],
  ])) {
    assert.equal(windowsShortcutFor(keyboardEvent({ key, ctrlKey: true, altKey: true }), "windows"), shortcut);
  }
  assert.equal(
    windowsShortcutFor(keyboardEvent({ key: "z", ctrlKey: true, altKey: true, shiftKey: true }), "windows"),
    "editRedo",
  );
  assert.equal(windowsShortcutFor(keyboardEvent({ key: "t", ctrlKey: true }), "windows"), null);
  assert.equal(windowsShortcutFor(keyboardEvent({ key: "c", ctrlKey: true }), "windows"), null);
});

test("Windows application shortcuts run through the frontend menu", () => {
  const sidebarKey = keyboardEvent({ key: "B", ctrlKey: true, shiftKey: true });
  assert.equal(windowsShortcutFor(sidebarKey, "windows"), "toggleSidebar");
  assert.equal(pageApplicationShortcutFor(sidebarKey, "windows"), "toggleSidebar");
  assert.equal(pageApplicationShortcutFor(keyboardEvent({ key: "c", ctrlKey: true, shiftKey: true }), "windows"), "editCopy");
  assert.equal(pageApplicationShortcutFor(keyboardEvent({ key: "z", ctrlKey: true, altKey: true }), "windows"), "editUndo");
  assert.equal(pageApplicationShortcutFor(sidebarKey, "linux"), "toggleSidebar");
  assert.equal(pageApplicationShortcutFor(keyboardEvent({ key: "c", ctrlKey: true }), "windows"), null);
  assert.equal(pageApplicationShortcutFor(keyboardEvent({ key: ",", ctrlKey: true }), "windows"), "settings");
  assert.equal(pageApplicationShortcutFor(keyboardEvent({ key: ",", ctrlKey: true }), "linux"), "settings");
  assert.equal(windowsShortcutFor(keyboardEvent({ key: ",", ctrlKey: true }), "windows"), "settings");
  assert.equal(linuxShortcutFor(keyboardEvent({ key: ",", ctrlKey: true }), "linux"), "settings");
  assert.equal(linuxShortcutFor(keyboardEvent({ key: ",", ctrlKey: true, shiftKey: true }), "linux"), null);
  assert.equal(windowsShortcutFor(keyboardEvent({ key: ",", ctrlKey: true, altKey: true }), "windows"), null);
});

test("font aliases follow the platform primary modifier", () => {
  assert.equal(terminalFontCommandFor(keyboardEvent({ key: "=", ctrlKey: true }), "linux"), "increase");
  assert.equal(terminalFontCommandFor(keyboardEvent({ key: "-", ctrlKey: true }), "linux"), "decrease");
  assert.equal(terminalFontCommandFor(keyboardEvent({ key: "0", metaKey: true }), "linux"), null);
  assert.equal(terminalFontCommandFor(keyboardEvent({ key: "=", metaKey: true }), "mac"), "increase");
});

test("Linux application shortcuts map once and preserve terminal input", () => {
  const shifted = new Map([
    ["n", "newTab"],
    ["t", "newLocalTerminal"],
    ["p", "newSFTP"],
    ["w", "closeTab"],
    ["b", "toggleSidebar"],
    ["c", "editCopy"],
    ["v", "editPaste"],
    ["x", "editCut"],
    ["a", "editSelectAll"],
  ]);
  for (const [key, shortcut] of shifted) {
    assert.equal(linuxShortcutFor(keyboardEvent({ key, ctrlKey: true, shiftKey: true }), "linux"), shortcut);
  }
  for (const [key, shortcut] of new Map([
    ["n", "newConnection"],
    ["m", "minimise"],
    ["q", "quit"],
    ["z", "editUndo"],
  ])) {
    assert.equal(linuxShortcutFor(keyboardEvent({ key, ctrlKey: true, altKey: true }), "linux"), shortcut);
  }
  assert.equal(linuxShortcutFor(keyboardEvent({ key: "z", ctrlKey: true, altKey: true, shiftKey: true }), "linux"), "editRedo");
  assert.equal(linuxShortcutFor(keyboardEvent({ key: "+", code: "Equal", ctrlKey: true, shiftKey: true }), "linux"), "increaseFont");
  assert.equal(linuxShortcutFor(keyboardEvent({ key: "=", code: "Equal", ctrlKey: true }), "linux"), "increaseFont");
  assert.equal(linuxShortcutFor(keyboardEvent({ key: "-", ctrlKey: true }), "linux"), "decreaseFont");
  assert.equal(linuxShortcutFor(keyboardEvent({ key: "0", ctrlKey: true }), "linux"), "resetFont");
  assert.equal(linuxShortcutFor(keyboardEvent({ key: "t", ctrlKey: true }), "linux"), null);
  assert.equal(linuxShortcutFor(keyboardEvent({ key: "t", ctrlKey: true, shiftKey: true }), "mac"), null);
  assert.equal(linuxShortcutFor(keyboardEvent({ key: "d", ctrlKey: true }), "linux"), null);
});

test("primary modifier excludes terminal-safe alternate combinations", () => {
  assert.equal(usesPrimaryModifier(keyboardEvent({ ctrlKey: true }), "linux"), true);
  assert.equal(usesPrimaryModifier(keyboardEvent({ ctrlKey: true, altKey: true }), "linux"), false);
  assert.equal(usesPrimaryModifier(keyboardEvent({ metaKey: true }), "mac"), true);
  assert.equal(usesPrimaryModifier(keyboardEvent({ metaKey: true, ctrlKey: true }), "mac"), false);
});

test("shortcut labels expose the native modifier for each desktop", () => {
  assert.equal(shortcutLabel("⌘T", "Ctrl+Shift+N", "mac"), "⌘T");
  assert.equal(shortcutLabel("⌘T", "Ctrl+Shift+N", "linux"), "Ctrl+Shift+N");
  assert.equal(shortcutLabel("⌘T", "Ctrl+Shift+N", "windows"), "Ctrl+Shift+N");
});

test("macOS application shortcuts take precedence over terminal editing mappings", () => {
  const tabShortcut = keyboardEvent({ key: "1", metaKey: true });
  assert.deepEqual(tabCommandFor(tabShortcut, "mac"), { kind: "select", index: 0 });
  assert.equal(macTerminalEditingInputFor(tabShortcut, "mac", true), null);

  const fontShortcut = keyboardEvent({ key: "=", metaKey: true });
  assert.equal(terminalFontCommandFor(fontShortcut, "mac"), "increase");
  assert.equal(macTerminalEditingInputFor(fontShortcut, "mac", true), null);

  assert.equal(
    macTerminalEditingInputFor(keyboardEvent({ key: "c", metaKey: true }), "mac", true),
    null,
  );
});
