import type { ShortcutPlatform } from "./shortcuts";

export type TerminalKeyEvent = Pick<
  KeyboardEvent,
  | "type"
  | "key"
  | "shiftKey"
  | "ctrlKey"
  | "altKey"
  | "metaKey"
  | "isComposing"
  | "keyCode"
  | "repeat"
> & {
  preventDefault: () => void;
};

export type TerminalPathSyntax = "shell" | "sftp" | "powershell";
export const maximumDroppedPathBytes = 16 * 1024;
export const maximumTerminalPastePreviewCharacters = 240;

export interface TerminalInputEvent {
  sessionId: number;
  processInstanceId: number;
  input: string;
  // xterm.js emits terminal query replies through onData. They must be sent
  // back to the process that requested them without entering live broadcast.
  mirrorToBroadcast: boolean;
}

export interface TerminalInputSegment {
  input: string;
  mirrorToBroadcast: boolean;
}

export interface TerminalProcessTarget {
  sessionId: number;
  processInstanceId: number | null;
}

export interface TerminalPasteInfo {
  lineCount: number;
  byteCount: number;
  preview: string;
  requiresConfirmation: boolean;
}

export function utf8ByteLength(value: string): number {
  let bytes = 0;
  for (let index = 0; index < value.length; index++) {
    const code = value.charCodeAt(index);
    if (code < 0x80) {
      bytes++;
    } else if (code < 0x800) {
      bytes += 2;
    } else if (code >= 0xd800 && code <= 0xdbff && index + 1 < value.length) {
      const next = value.charCodeAt(index + 1);
      if (next >= 0xdc00 && next <= 0xdfff) {
        bytes += 4;
        index++;
      } else {
        bytes += 3;
      }
    } else {
      // TextEncoder replaces unpaired surrogates with U+FFFD.
      bytes += 3;
    }
  }
  return bytes;
}

// xterm.js reports answers to terminal queries through onData, alongside real
// keyboard input. Forwarding those answers to another PTY turns them into
// literal shell input (for example, `gh` asks for OSC 11 and CSI 6n). Keep
// this list limited to response forms emitted by xterm.js so ordinary escape
// sequences such as cursor keys and mouse reports remain usable.
const terminalResponsePatterns = [
  /^\x1b\](?:4;\d+|10|11|12);rgb:[0-9a-fA-F]{4}\/[0-9a-fA-F]{4}\/[0-9a-fA-F]{4}(?:\x07|\x1b\\)$/u,
  /^\x1b\[\?(?:\d+;)*\d+c$/u,
  /^\x1b\[>(?:\d+;)*\d+c$/u,
  /^\x1bP>\|xterm\.js\([^\)]{0,64}\)\x1b\\$/u,
  /^\x1b\[(?:\??\d+;)*\??\d+(?:R|n)$/u,
  /^\x1b\[\??\d+;\d+\$y$/u,
  /^\x1b\[\d+(?:;\d+)*t$/u,
  /^\x1b\[(?:I|O)$/u,
  /^\x1b\[\?997;[12]n$/u,
  /^\x1b\[\?\d+u$/u,
  /^\x1bP1\$r[^\x1b]{0,256}\x1b\\$/u,
];

const terminalResponsePrefixPatterns = terminalResponsePatterns.map(
  (pattern) => new RegExp(pattern.source.replace(/\$$/u, ""), "u"),
);
const maximumTerminalResponseLength = 512;
const maximumTerminalResponseSegments = 1024;

function terminalGeneratedResponseAt(input: string, offset: number): string | null {
  const candidate = input.slice(offset, offset + maximumTerminalResponseLength);
  for (let index = 0; index < terminalResponsePrefixPatterns.length; index++) {
    const match = terminalResponsePrefixPatterns[index].exec(candidate);
    if (match !== null && match.index === 0 && terminalResponsePatterns[index].test(match[0])) {
      return match[0];
    }
  }
  return null;
}

export function splitTerminalInput(input: string): TerminalInputSegment[] {
  if (input.length === 0) {
    return [];
  }

  const segments: TerminalInputSegment[] = [];
  let normalStart = 0;
  let searchOffset = 0;
  let responseCount = 0;
  while (responseCount < maximumTerminalResponseSegments) {
    const escapeOffset = input.indexOf("\x1b", searchOffset);
    if (escapeOffset < 0) {
      break;
    }
    const response = terminalGeneratedResponseAt(input, escapeOffset);
    if (response === null) {
      searchOffset = escapeOffset + 1;
      continue;
    }
    if (escapeOffset > normalStart) {
      segments.push({ input: input.slice(normalStart, escapeOffset), mirrorToBroadcast: true });
    }
    segments.push({ input: response, mirrorToBroadcast: false });
    responseCount++;
    normalStart = escapeOffset + response.length;
    searchOffset = normalStart;
  }
  if (normalStart < input.length) {
    segments.push({ input: input.slice(normalStart), mirrorToBroadcast: true });
  }
  return segments;
}

export function logicalLineCount(value: string): number {
  if (value.length === 0) {
    return 0;
  }
  let lines = 1;
  for (let index = 0; index < value.length; index++) {
    const code = value.charCodeAt(index);
    if (code === 13) {
      lines++;
      if (value.charCodeAt(index + 1) === 10) {
        index++;
      }
    } else if (code === 10) {
      lines++;
    }
  }
  return lines;
}

export function boundedPreview(value: string, maximumCharacters: number): string {
  if (maximumCharacters < 1) {
    return "";
  }
  const characters: string[] = [];
  for (const character of value) {
    if (characters.length === maximumCharacters) {
      return `${characters.slice(0, -1).join("")}…`;
    }
    characters.push(character);
  }
  return value;
}

export function terminalPasteRequiresConfirmation(value: string): boolean {
  return /[\r\n]/.test(value);
}

export function classifyTerminalPaste(value: string): TerminalPasteInfo {
  return {
    lineCount: logicalLineCount(value),
    byteCount: utf8ByteLength(value),
    preview: boundedPreview(value, maximumTerminalPastePreviewCharacters),
    requiresConfirmation: terminalPasteRequiresConfirmation(value),
  };
}

export function terminalPastePrompt(sessionLabel: string, lineCount: number, recipientCount: number | null): string {
  if (recipientCount === null) {
    return `Paste into ${sessionLabel}?`;
  }
  return `Paste ${lineCount} line${lineCount === 1 ? "" : "s"} into ${recipientCount} sessions?`;
}

export function isTerminalTargetCurrent(
  target: TerminalProcessTarget,
  currentSessionId: number,
  currentProcessInstanceId: number | null,
): boolean {
  return target.sessionId === currentSessionId && target.processInstanceId === currentProcessInstanceId;
}

export type TerminalRightClickAction = "menu" | "paste" | "terminal";

// Mouse-reporting programs own ordinary clicks. Shift reaches the app menu.
export function terminalRightClickAction(
  mouseTrackingMode: string,
  shiftKey: boolean,
  rightClickToPaste: boolean,
  live: boolean,
): TerminalRightClickAction {
  if (shiftKey) {
    return "menu";
  }
  if (mouseTrackingMode !== "none") {
    return "terminal";
  }
  return rightClickToPaste && live ? "paste" : "menu";
}

const shiftEnterInput = "\n";

function isUnmodifiedShiftEnter(event: TerminalKeyEvent): boolean {
  return (
    event.key === "Enter" &&
    event.shiftKey &&
    !event.ctrlKey &&
    !event.altKey &&
    !event.metaKey
  );
}

// xterm.js emits CR for Enter regardless of Shift. The app maps its newline
// action to Ctrl+J, whose raw terminal input is LF.
export function handleShiftEnter(
  event: TerminalKeyEvent,
  sendInput: (input: string) => void,
): boolean {
  if (!isUnmodifiedShiftEnter(event)) {
    return true;
  }

  event.preventDefault();
  if (event.type === "keydown") {
    sendInput(shiftEnterInput);
  }
  return false;
}

// Local Command+Arrow sends xterm Home/End so shells and full-screen programs
// receive keys instead of bytes tied to one editing keymap. Remote sessions
// retain their existing bytes. Word editing stays as Meta input.
export function macTerminalEditingInputFor(
  event: TerminalKeyEvent,
  platform: ShortcutPlatform,
  localShell: boolean,
): string | null {
  if (
    platform !== "mac" ||
    (event.type !== "keydown" && event.type !== "keypress" && event.type !== "keyup") ||
    event.isComposing ||
    event.keyCode === 229 ||
    event.ctrlKey ||
    event.shiftKey
  ) {
    return null;
  }

  if (event.metaKey && !event.altKey) {
    switch (event.key) {
      case "ArrowLeft":
        return localShell ? "\x1bOH" : "\x01";
      case "ArrowRight":
        return localShell ? "\x1bOF" : "\x05";
      case "Backspace":
        return "\x15";
      default:
        return null;
    }
  }

  if (event.altKey && !event.metaKey) {
    switch (event.key) {
      case "ArrowLeft":
        return "\x1bb";
      case "ArrowRight":
        return "\x1bf";
      case "Backspace":
        return "\x1b\x7f";
      default:
        return null;
    }
  }

  return null;
}

// xterm invokes its custom key handler for more than keydown. Consume every
// matching phase, but emit only on keydown so keypress/keyup cannot duplicate
// input. Repeated keydown events intentionally emit repeated input.
export function handleMacTerminalEditing(
  event: TerminalKeyEvent,
  sendInput: (input: string) => void,
  platform: ShortcutPlatform,
  localShell: boolean,
): boolean {
  const input = macTerminalEditingInputFor(event, platform, localShell);
  if (input === null) {
    return true;
  }

  event.preventDefault();
  if (event.type === "keydown") {
    sendInput(input);
  }
  return false;
}

function quoteShellPath(path: string): string {
  return `'${path.replaceAll("'", "'\\''")}'`;
}

function quoteSFTPPath(path: string): string {
  // Unlike a shell, sftp consumes backslashes even inside single quotes.
  // Its parser also escapes glob metacharacters found inside quoted paths.
  return `"${path.replaceAll("\\", "\\\\").replaceAll('"', '\\"')}"`;
}

function quotePowerShellPath(path: string): string {
  // PowerShell's single-quoted strings escape a literal apostrophe by doubling
  // it; backslashes remain ordinary path characters.
  return `'${path.replaceAll("'", "''")}'`;
}

function isWindowsAbsolutePath(path: string): boolean {
  return /^[A-Za-z]:[\\/]/.test(path) || path.startsWith("\\\\") || path.startsWith("//");
}

export function formatDroppedPaths(
  paths: readonly string[],
  syntax: TerminalPathSyntax,
): string {
  const quotePath = syntax === "sftp"
    ? quoteSFTPPath
    : syntax === "powershell"
      ? quotePowerShellPath
      : quoteShellPath;
  const quoted: string[] = [];
  let bytes = 0;
  for (const path of paths) {
    const hasControl = /[\u0000-\u001f\u007f-\u009f]/.test(path);
    const posixAbsolute = path.startsWith("/");
    const windowsAbsolute = isWindowsAbsolutePath(path);
    const validAbsolute = syntax === "powershell"
      ? windowsAbsolute
      : syntax === "sftp"
        ? posixAbsolute || windowsAbsolute
        : posixAbsolute && !windowsAbsolute;
    if (hasControl || !validAbsolute) {
      if (syntax === "shell" && windowsAbsolute && !hasControl) {
        throw new Error("Windows paths cannot be dropped into an SSH terminal because the remote shell is unknown.");
      }
      throw new Error("Dropped paths must be absolute and cannot contain control characters.");
    }
    const value = quotePath(path);
    bytes += new TextEncoder().encode(value).byteLength + (quoted.length > 0 ? 1 : 0);
    if (bytes > maximumDroppedPathBytes) {
      throw new Error("Dropped paths exceed the 16 KB input limit.");
    }
    quoted.push(value);
  }
  return quoted.join(" ");
}
