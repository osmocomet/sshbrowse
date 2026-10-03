import {
  maximumWorkspaceSessions,
  sessionDisplayLabel,
  sessionDisplayLabels,
  sessionIds,
  type Session,
  type Tab,
  type WorkspaceTab,
} from "./tabs.ts";
import { boundedPreview, logicalLineCount, utf8ByteLength } from "./terminalInput.ts";

export type BroadcastScope = "current" | "all";

export interface SessionPlacement {
  tabs: Tab[];
  activeId: number;
  nextTabId: number;
}

export interface BroadcastRecipient {
  logicalSessionId: number;
  processInstanceId: number | null;
  label: string;
  status: Session["status"];
  available: boolean;
}

export interface BroadcastSnapshot {
  logicalSessionId: number;
  processInstanceId: number;
  label: string;
}

export interface BroadcastDelivery extends BroadcastSnapshot {
  error: string | null;
}

export const maximumBroadcastCommandBytes = 16 * 1024;
export const maximumBroadcastCommandPreviewCharacters = 240;
export const maximumParallelBroadcastWrites = 4;
export const defaultBroadcastSendDeadlineMs = 10_000;
export const broadcastDeliveryUnknownError = "Delivery status unknown: the write did not finish before the send deadline.";
export const broadcastDeliverySkippedError = "Not sent: the send deadline expired before this session was reached.";

export function normalizeBroadcastCommand(command: string): string {
  const normalized = command.replace(/\r\n|\r|\n/g, "\r");
  return `${normalized.replace(/\r+$/, "")}\r`;
}

export function broadcastCommandLineCount(command: string): number {
  const normalized = normalizeBroadcastCommand(command).slice(0, -1);
  return logicalLineCount(normalized);
}

export function broadcastCommandPreview(command: string): string {
  return boundedPreview(command, maximumBroadcastCommandPreviewCharacters);
}

export function placeNewSession(
  tabs: Tab[],
  activeId: number | null,
  sessionId: number,
  tilingMode: boolean,
  nextTabId: number,
): SessionPlacement {
  const activeIndex = tabs.findIndex((tab) => tab.id === activeId);
  if (!tilingMode || activeIndex < 0) {
    const tab: Tab = { id: nextTabId, kind: "session", sessionId };
    return { tabs: [...tabs, tab], activeId: tab.id, nextTabId: nextTabId + 1 };
  }

  const activeTab = tabs[activeIndex];
  if (activeTab.kind === "session") {
    const workspace: WorkspaceTab = {
      id: activeTab.id,
      kind: "workspace",
      sessionIds: [activeTab.sessionId, sessionId],
      selectedSessionId: sessionId,
    };
    return {
      tabs: tabs.map((tab, index) => index === activeIndex ? workspace : tab),
      activeId: workspace.id,
      nextTabId,
    };
  }

  if (activeTab.sessionIds.length < maximumWorkspaceSessions) {
    const workspace: WorkspaceTab = {
      ...activeTab,
      sessionIds: [...activeTab.sessionIds, sessionId],
      selectedSessionId: sessionId,
    };
    return {
      tabs: tabs.map((tab, index) => index === activeIndex ? workspace : tab),
      activeId: workspace.id,
      nextTabId,
    };
  }

  const tab: Tab = { id: nextTabId, kind: "session", sessionId };
  return { tabs: [...tabs, tab], activeId: tab.id, nextTabId: nextTabId + 1 };
}

export function broadcastRecipients(
  sessions: Session[],
  tabs: Tab[],
  activeId: number | null,
  scope: BroadcastScope,
  labels: ReadonlyMap<number, string> = sessionDisplayLabels(sessions),
): BroadcastRecipient[] {
  const scopedTab = tabs.find((tab) => tab.id === activeId);
  const includedIds = scope === "all"
    ? tabs.flatMap(sessionIds)
    : scopedTab ? sessionIds(scopedTab) : [];
  const sessionsById = new Map(sessions.map((session) => [session.id, session]));
  const includedSessions = includedIds
    .map((id) => sessionsById.get(id))
    .filter((session): session is Session => session !== undefined);

  return includedSessions.map((session) => {
    return {
      logicalSessionId: session.id,
      processInstanceId: session.processInstanceId,
      label: sessionDisplayLabel(labels, session.id),
      status: session.status,
      available: session.status === "live" && session.processInstanceId !== null,
    };
  });
}

export function snapshotBroadcastRecipients(
  recipients: BroadcastRecipient[],
  excludedSessionIds: ReadonlySet<number>,
): BroadcastSnapshot[] {
  return recipients.flatMap((recipient) => {
    if (!recipient.available || recipient.processInstanceId === null || excludedSessionIds.has(recipient.logicalSessionId)) {
      return [];
    }
    return [{
      logicalSessionId: recipient.logicalSessionId,
      processInstanceId: recipient.processInstanceId,
      label: recipient.label,
    }];
  });
}

export function broadcastCommandError(command: string): string | null {
  if (command.trim().length === 0) {
    return "Enter a command to send.";
  }
  if (/[\u0000-\u0009\u000b\u000c\u000e-\u001f\u007f-\u009f]/u.test(command)) {
    return "The command cannot contain terminal control characters.";
  }
  if (utf8ByteLength(command) > maximumBroadcastCommandBytes) {
    return `Command is limited to ${maximumBroadcastCommandBytes / 1024} KB.`;
  }
  return null;
}

export interface BroadcastSender {
  send(recipients: readonly BroadcastSnapshot[], command: string): Promise<BroadcastDelivery[]>;
  isBusy(): boolean;
}

export interface BroadcastSenderOptions {
  deadlineMs?: number;
}

export function createBroadcastSender(
  write: (processInstanceId: number, data: string) => Promise<void>,
  options: BroadcastSenderOptions = {},
): BroadcastSender {
  const deadlineMs = options.deadlineMs ?? defaultBroadcastSendDeadlineMs;
  if (!Number.isFinite(deadlineMs) || deadlineMs <= 0) {
    throw new RangeError("Broadcast send deadline must be finite and positive.");
  }

  let busy = false;

  return {
    async send(recipients, command) {
      if (busy) {
        throw new Error("A previous broadcast is still finishing. No new input was sent.");
      }
      const validationError = broadcastCommandError(command);
      if (validationError !== null) throw new Error(validationError);
      const data = normalizeBroadcastCommand(command);
      busy = true;
      const snapshot = recipients.map((recipient) => ({ ...recipient }));
      const deliveries: BroadcastDelivery[] = new Array(snapshot.length);
      let nextIndex = 0;
      let expired = false;
      const workers = Array.from({ length: Math.min(maximumParallelBroadcastWrites, snapshot.length) }, async () => {
        while (!expired && nextIndex < snapshot.length) {
          const index = nextIndex++;
          const recipient = snapshot[index];
          let error: string | null = null;
          try {
            await write(recipient.processInstanceId, data);
          } catch (writeError) {
            error = String(writeError);
          }
          if (!expired) deliveries[index] = { ...recipient, error };
        }
      });
      // A deadline cannot cancel a PTY write. Keep the sender occupied until
      // issued writes settle, including when the command bar closes and reopens.
      const completed = Promise.all(workers).then(() => {
        busy = false;
        return deliveries;
      });
      let deadlineTimer: ReturnType<typeof setTimeout>;
      const deadline = new Promise<BroadcastDelivery[]>((resolve) => {
        deadlineTimer = setTimeout(() => {
          expired = true;
          resolve(snapshot.map((recipient, index) => deliveries[index] ?? {
            ...recipient,
            error: index < nextIndex ? broadcastDeliveryUnknownError : broadcastDeliverySkippedError,
          }));
        }, deadlineMs);
      });
      try {
        return await Promise.race([completed, deadline]);
      } finally {
        clearTimeout(deadlineTimer!);
      }
    },
    isBusy() {
      return busy;
    },
  };
}
