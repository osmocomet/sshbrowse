import assert from "node:assert/strict";
import test from "node:test";

import {
  boundedPreview,
  classifyTerminalPaste,
  formatDroppedPaths,
  handleMacTerminalEditing,
  handleShiftEnter,
  macTerminalEditingInputFor,
  maximumDroppedPathBytes,
  isTerminalTargetCurrent,
  logicalLineCount,
  splitTerminalInput,
  terminalPasteRequiresConfirmation,
  terminalRightClickAction,
  utf8ByteLength,
  type TerminalKeyEvent,
} from "../src/lib/terminalInput.ts";

test("splits adjacent terminal replies without changing input order", () => {
  const colorReply = "\x1b]11;rgb:1919/1e1e/2121\x1b\\";
  const cursorReply = "\x1b[6;1R";

  assert.deepEqual(splitTerminalInput(`echo${colorReply}${cursorReply} ok\r`), [
    { input: "echo", mirrorToBroadcast: true },
    { input: colorReply, mirrorToBroadcast: false },
    { input: cursorReply, mirrorToBroadcast: false },
    { input: " ok\r", mirrorToBroadcast: true },
  ]);
});

test("does not misclassify an ordinary escape sequence before a reply", () => {
  const colorReply = "\x1b]11;rgb:1919/1e1e/2121\x1b\\";

  assert.deepEqual(splitTerminalInput(`\x1b[A${colorReply}`), [
    { input: "\x1b[A", mirrorToBroadcast: true },
    { input: colorReply, mirrorToBroadcast: false },
  ]);
});

function keyboardEvent(overrides: Partial<TerminalKeyEvent> = {}): TerminalKeyEvent & {
  defaultPrevented: boolean;
} {
  const event = {
    type: "keydown",
    key: "Enter",
    shiftKey: true,
    ctrlKey: false,
    altKey: false,
    metaKey: false,
    isComposing: false,
    keyCode: 0,
    repeat: false,
    defaultPrevented: false,
    preventDefault() {
      this.defaultPrevented = true;
    },
    ...overrides,
  };
  return event;
}

function createInputRecorder() {
  const receivedBytes: number[] = [];
  return {
    receivedBytes,
    send(input: string) {
      receivedBytes.push(...new TextEncoder().encode(input));
    },
  };
}

test("records one LF byte for Shift+Enter", () => {
  const recorder = createInputRecorder();
  const event = keyboardEvent();

  assert.equal(handleShiftEnter(event, recorder.send), false);
  assert.equal(event.defaultPrevented, true);
  assert.deepEqual(recorder.receivedBytes, [0x0a]);
});

test("does not emit a second byte for keypress or keyup", () => {
  const recorder = createInputRecorder();

  assert.equal(handleShiftEnter(keyboardEvent({ type: "keypress" }), recorder.send), false);
  assert.equal(handleShiftEnter(keyboardEvent({ type: "keyup" }), recorder.send), false);
  assert.deepEqual(recorder.receivedBytes, []);
});

test("leaves ordinary Enter unchanged", () => {
  const recorder = createInputRecorder();
  const event = keyboardEvent({ shiftKey: false });

  assert.equal(handleShiftEnter(event, recorder.send), true);
  assert.equal(event.defaultPrevented, false);
  assert.deepEqual(recorder.receivedBytes, []);
});

test("leaves modified shortcuts and Option-as-Meta unchanged", () => {
  for (const modifier of ["ctrlKey", "altKey", "metaKey"] as const) {
    const recorder = createInputRecorder();
    const event = keyboardEvent({ [modifier]: true });

    assert.equal(handleShiftEnter(event, recorder.send), true, modifier);
    assert.equal(event.defaultPrevented, false, modifier);
    assert.deepEqual(recorder.receivedBytes, [], modifier);
  }
});

test("does not intercept paste input", () => {
  const recorder = createInputRecorder();
  const event = keyboardEvent({ type: "input", key: "", shiftKey: false });

  assert.equal(handleShiftEnter(event, recorder.send), true);
  assert.deepEqual(recorder.receivedBytes, []);
});

test("encodes Command arrows as xterm Home/End and Option arrows as Meta words", () => {
  const cases = [
    { key: "ArrowLeft", metaKey: true, shiftKey: false, want: [0x1b, 0x4f, 0x48] },
    { key: "ArrowRight", metaKey: true, shiftKey: false, want: [0x1b, 0x4f, 0x46] },
    { key: "Backspace", metaKey: true, shiftKey: false, want: [0x15] },
    { key: "ArrowLeft", altKey: true, shiftKey: false, want: [0x1b, 0x62] },
    { key: "ArrowRight", altKey: true, shiftKey: false, want: [0x1b, 0x66] },
    { key: "Backspace", altKey: true, shiftKey: false, want: [0x1b, 0x7f] },
  ];

  for (const { want, ...overrides } of cases) {
    const recorder = createInputRecorder();
    const event = keyboardEvent(overrides);

    assert.equal(handleMacTerminalEditing(event, recorder.send, "mac", true), false, JSON.stringify(overrides));
    assert.equal(event.defaultPrevented, true, JSON.stringify(overrides));
    assert.deepEqual(recorder.receivedBytes, want, JSON.stringify(overrides));
  }
});

test("keeps remote Command arrow bytes unchanged", () => {
  assert.equal(macTerminalEditingInputFor(keyboardEvent({ key: "ArrowLeft", shiftKey: false, metaKey: true }), "mac", false), "\x01");
  assert.equal(macTerminalEditingInputFor(keyboardEvent({ key: "ArrowRight", shiftKey: false, metaKey: true }), "mac", false), "\x05");
});

test("consumes all matching event phases without duplicating input, while retaining repeat", () => {
  const recorder = createInputRecorder();
  const keydown = keyboardEvent({ key: "ArrowLeft", altKey: true, shiftKey: false });
  const keypress = keyboardEvent({ key: "ArrowLeft", altKey: true, shiftKey: false, type: "keypress" });
  const keyup = keyboardEvent({ key: "ArrowLeft", altKey: true, shiftKey: false, type: "keyup" });
  const repeatedKeydown = keyboardEvent({ key: "ArrowLeft", altKey: true, shiftKey: false, repeat: true });

  assert.equal(handleMacTerminalEditing(keydown, recorder.send, "mac", true), false);
  assert.equal(handleMacTerminalEditing(keypress, recorder.send, "mac", true), false);
  assert.equal(handleMacTerminalEditing(keyup, recorder.send, "mac", true), false);
  assert.equal(handleMacTerminalEditing(repeatedKeydown, recorder.send, "mac", true), false);
  assert.deepEqual(recorder.receivedBytes, [0x1b, 0x62, 0x1b, 0x62]);
  assert.equal(keypress.defaultPrevented, true);
  assert.equal(keyup.defaultPrevented, true);
});

test("gates macOS editing on platform, composition, and exact modifiers", () => {
  const blocked = [
    keyboardEvent({ key: "ArrowLeft", altKey: true, shiftKey: false, isComposing: true }),
    keyboardEvent({ key: "ArrowLeft", altKey: true, shiftKey: false, keyCode: 229 }),
    keyboardEvent({ key: "ArrowLeft", altKey: true, shiftKey: false, ctrlKey: true }),
    keyboardEvent({ key: "ArrowLeft", altKey: true, shiftKey: false, metaKey: true }),
    keyboardEvent({ key: "ArrowLeft", altKey: true, shiftKey: true }),
  ];

  for (const event of blocked) {
    assert.equal(macTerminalEditingInputFor(event, "mac", true), null);
    assert.equal(event.defaultPrevented, false);
  }

  for (const platform of ["linux", "windows"] as const) {
    const event = keyboardEvent({ key: "ArrowLeft", shiftKey: false, metaKey: true });
    assert.equal(macTerminalEditingInputFor(event, platform, true), null);
    assert.equal(handleMacTerminalEditing(event, () => assert.fail("must not emit"), platform, true), true);
  }
});

test("classifies short single-line terminal pastes as immediate", () => {
  assert.equal(terminalPasteRequiresConfirmation("docker ps"), false);
  assert.deepEqual(classifyTerminalPaste("docker ps"), {
    lineCount: 1,
    byteCount: 9,
    preview: "docker ps",
    requiresConfirmation: false,
  });
});

test("requires confirmation for multiline terminal pastes", () => {
  assert.equal(terminalPasteRequiresConfirmation("sudo dnf update\nsudo systemctl restart nginx"), true);
  assert.equal(terminalPasteRequiresConfirmation("sudo dnf update\r\nsudo systemctl restart nginx"), true);
  assert.equal(terminalPasteRequiresConfirmation("a".repeat(4096)), false);
  assert.equal(terminalPasteRequiresConfirmation("a".repeat(4097)), false);
  assert.equal(classifyTerminalPaste("é".repeat(4097)).requiresConfirmation, false);
});

test("counts mixed terminal line endings without changing their meaning", () => {
  assert.equal(logicalLineCount(""), 0);
  assert.equal(logicalLineCount("a\r\nb\rc\nd\r\n"), 5);
});

test("counts UTF-8 bytes like TextEncoder, including malformed surrogates", () => {
  for (const value of ["", "plain text", "é中😀", "\ud800", "\udc00", "\ud800A", "A\udc00", "\ud800\udc00"]) {
    assert.equal(utf8ByteLength(value), new TextEncoder().encode(value).byteLength, JSON.stringify(value));
  }
});

test("limits paste previews by Unicode characters", () => {
  assert.equal(boundedPreview("a😀é", 3), "a😀é");
  assert.equal(boundedPreview("a😀éz", 3), "a😀…");
  assert.equal(boundedPreview("😀".repeat(241), 240), `${"😀".repeat(239)}…`);
});

test("does not approve a delayed paste for a changed process instance", () => {
  const target = { sessionId: 7, processInstanceId: 42 };
  assert.equal(isTerminalTargetCurrent(target, 7, 42), true);
  assert.equal(isTerminalTargetCurrent(target, 7, 43), false);
  assert.equal(isTerminalTargetCurrent(target, 8, 42), false);
  assert.equal(isTerminalTargetCurrent({ sessionId: 7, processInstanceId: null }, 7, null), true);
});

test("leaves existing Control editing and application-owned Command input untouched", () => {
  for (const event of [
    keyboardEvent({ key: "a", shiftKey: false, ctrlKey: true }),
    keyboardEvent({ key: "e", shiftKey: false, ctrlKey: true }),
    keyboardEvent({ key: "u", shiftKey: false, ctrlKey: true }),
    keyboardEvent({ key: "w", shiftKey: false, ctrlKey: true }),
    keyboardEvent({ key: "k", shiftKey: false, ctrlKey: true }),
    keyboardEvent({ key: "c", shiftKey: false, metaKey: true }),
    keyboardEvent({ key: "1", shiftKey: false, metaKey: true }),
    keyboardEvent({ key: "ArrowLeft", shiftKey: false, metaKey: true, type: "input" }),
  ]) {
    assert.equal(macTerminalEditingInputFor(event, "mac", true), null, `${event.key} should pass through`);
  }
});

test("right-click paste leaves mouse reporting and closed terminals alone", () => {
  assert.equal(terminalRightClickAction("none", false, false, true), "menu");
  assert.equal(terminalRightClickAction("none", false, true, true), "paste");
  assert.equal(terminalRightClickAction("none", false, true, false), "menu");
  assert.equal(terminalRightClickAction("any", false, true, true), "terminal");
  assert.equal(terminalRightClickAction("any", true, true, true), "menu");
  assert.equal(terminalRightClickAction("none", true, true, true), "menu");
});

test("quotes multiple shell paths without adding command input", () => {
  assert.equal(
    formatDroppedPaths(
      ["/tmp/space name", "/tmp/quote'name", "/tmp/back\\slash"],
      "shell",
    ),
    "'/tmp/space name' '/tmp/quote'\\''name' '/tmp/back\\slash'",
  );
});

test("quotes sftp paths using its own parser syntax", () => {
  assert.equal(
    formatDroppedPaths(
      ["/tmp/space name", "/tmp/quote'name", '/tmp/quote"name', "/tmp/back\\slash", "/tmp/file[*]?"],
      "sftp",
    ),
    '"/tmp/space name" "/tmp/quote\'name" "/tmp/quote\\"name" "/tmp/back\\\\slash" "/tmp/file[*]?"',
  );
});

test("quotes Windows PowerShell paths and accepts drive and UNC roots", () => {
  assert.equal(
    formatDroppedPaths(["C:\\Users\\Core User\\notes.txt", "\\\\server\\Share\\O'Brien.txt"], "powershell"),
    "'C:\\Users\\Core User\\notes.txt' '\\\\server\\Share\\O''Brien.txt'",
  );
});

test("SFTP accepts Windows local paths while an SSH shell rejects them", () => {
  assert.equal(
    formatDroppedPaths(["C:\\Users\\Core User\\notes.txt"], "sftp"),
    String.raw`"C:\\Users\\Core User\\notes.txt"`,
  );
  for (const path of ["C:\\Users\\Core User\\notes.txt", "\\\\server\\Share\\notes.txt"]) {
    assert.throws(() => formatDroppedPaths([path], "shell"), /remote shell is unknown/);
  }
});

test("rejects the whole drop when any path contains terminal controls or is relative", () => {
  for (const syntax of ["shell", "sftp"] as const) {
    for (const invalid of ["report.txt", "/tmp/line\nfeed", "/tmp/tab\tfile", "/tmp/escape\x1b", "/tmp/csi\u009b"]) {
      assert.throws(() => formatDroppedPaths(["/tmp/ok", invalid], syntax), /absolute.*control/);
    }
  }
});

test("bounds quoted UTF-8 input including separators and quote expansion", () => {
  assert.equal(formatDroppedPaths([], "shell"), "");
  const fits = "/" + "a".repeat(maximumDroppedPathBytes - 3);
  assert.equal(new TextEncoder().encode(formatDroppedPaths([fits], "shell")).byteLength, maximumDroppedPathBytes);
  assert.throws(() => formatDroppedPaths([fits, "/another"], "shell"), /16 KB/);
  assert.throws(() => formatDroppedPaths(["/" + "é".repeat(maximumDroppedPathBytes / 2)], "sftp"), /16 KB/);
  assert.throws(() => formatDroppedPaths(["/" + "'".repeat(maximumDroppedPathBytes / 3)], "shell"), /16 KB/);
});
