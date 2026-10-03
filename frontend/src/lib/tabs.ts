import type { Connection } from "../../bindings/sshbrowse/internal/profile/models";

export type SessionStatus = "connecting" | "live" | "closed";
export const maximumWorkspaceSessions = 9;
export const maximumTabs = 20;

// Which OpenSSH program runs against the connection. Ignored for a local shell.
export type SessionCommand = "ssh" | "sftp";

export interface Session {
  id: number;
  // null runs a local shell; otherwise a snapshot of the saved or unsaved SSH connection.
  connection: Connection | null;
  command: SessionCommand;
  status: SessionStatus;
  // This backend session key belongs to TerminalPane and changes on reconnect.
  processInstanceId: number | null;
}

function sessionTitle(session: Session): string {
  if (session.connection === null) {
    return "Local terminal";
  }
  return session.command === "sftp" ? `${session.connection.name} (sftp)` : session.connection.name;
}

export function sessionDisplayLabels(sessions: readonly Session[]): Map<number, string> {
  const sessionsByTitle = new Map<string, Session[]>();
  for (const session of sessions) {
    const title = sessionTitle(session);
    const matchingSessions = sessionsByTitle.get(title);
    if (matchingSessions) {
      matchingSessions.push(session);
    } else {
      sessionsByTitle.set(title, [session]);
    }
  }

  const candidateLabels = new Map<number, string>();
  const sessionsByCandidateLabel = new Map<string, Session[]>();
  for (const [title, matchingSessions] of sessionsByTitle) {
    const orderedSessions = [...matchingSessions].sort((left, right) => left.id - right.id);
    orderedSessions.forEach((session, index) => {
      const candidateLabel = orderedSessions.length === 1 ? title : `${title} (${index + 1})`;
      candidateLabels.set(session.id, candidateLabel);
      const collidingSessions = sessionsByCandidateLabel.get(candidateLabel);
      if (collidingSessions) {
        collidingSessions.push(session);
      } else {
        sessionsByCandidateLabel.set(candidateLabel, [session]);
      }
    });
  }

  // Keep every candidate occupied while adding qualifiers. A literal name can
  // otherwise occupy the exact label that a duplicate would use for its first
  // qualifier, such as "a (1) [#1]".
  const occupiedLabels = new Set(candidateLabels.values());
  // At most 2n - 1 candidate or earlier resolved labels can be occupied.
  const maximumQualifierAttempt = sessions.length * 2;
  const resolvedLabels = new Map<number, string>();
  const candidateGroups = [...sessionsByCandidateLabel.entries()].sort(([left], [right]) =>
    left < right ? -1 : left > right ? 1 : 0,
  );
  for (const [candidateLabel, collidingSessions] of candidateGroups) {
    const orderedSessions = [...collidingSessions].sort((left, right) => left.id - right.id);
    if (orderedSessions.length === 1) {
      resolvedLabels.set(orderedSessions[0].id, candidateLabel);
      continue;
    }

    for (const session of orderedSessions) {
      let qualifiedLabel = "";
      for (let attempt = 0; attempt <= maximumQualifierAttempt; attempt += 1) {
        const suffix = attempt === 0 ? `[#${session.id}]` : `[#${session.id}.${attempt + 1}]`;
        const candidate = `${candidateLabel} ${suffix}`;
        if (!occupiedLabels.has(candidate)) {
          qualifiedLabel = candidate;
          break;
        }
      }
      if (qualifiedLabel === "") {
        throw new Error("Could not find a unique session display label.");
      }
      resolvedLabels.set(session.id, qualifiedLabel);
      occupiedLabels.add(qualifiedLabel);
    }
  }

  // Keep the map's insertion order aligned with the session collection. The
  // labels themselves are global and deterministic; callers should not see
  // ordering changes just because collision groups were resolved separately.
  const labels = new Map<number, string>();
  for (const [sessionId] of candidateLabels) {
    const label = resolvedLabels.get(sessionId);
    if (label === undefined) {
      throw new Error("Could not resolve a session display label.");
    }
    labels.set(sessionId, label);
  }
  return labels;
}

export function sessionDisplayLabel(labels: ReadonlyMap<number, string>, sessionId: number): string {
  return labels.get(sessionId) ?? `Session [#${sessionId}]`;
}

export function sessionStatusLabel(status: SessionStatus): string {
  switch (status) {
    case "connecting":
      return "Connecting";
    case "live":
      return "Live";
    case "closed":
      return "Closed";
  }
}

export interface SessionTab {
  id: number;
  kind: "session";
  sessionId: number;
}

export interface WorkspaceTab {
  id: number;
  kind: "workspace";
  sessionIds: number[];
  selectedSessionId: number;
}

export type Tab = SessionTab | WorkspaceTab;

export function sessionIds(tab: Tab): number[] {
  return tab.kind === "session" ? [tab.sessionId] : tab.sessionIds;
}
