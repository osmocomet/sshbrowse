import {
  maximumWorkspaceSessions,
  sessionDisplayLabel,
  sessionIds,
  type Session,
  type Tab,
  type WorkspaceTab,
} from "./tabs.ts";

export type CloseTarget = { kind: "tab"; id: number } | { kind: "session"; id: number };

export interface CloseConfirmationRequest {
  target: CloseTarget;
  sessionIds: number[];
  sessionNames: string[];
  workspace: boolean;
}

export interface WorkspaceState<S extends Session = Session> {
  tabs: Tab[];
  sessions: S[];
  activeId: number | null;
}

export interface TabTransition {
  tabs: Tab[];
  activeId: number;
  nextTabId: number;
}

export interface SelectionTransition {
  tabs: Tab[];
  activeId: number;
}

export function closeTab<S extends Session>(
  state: WorkspaceState<S>,
  tabId: number,
): WorkspaceState<S> | null {
  const index = state.tabs.findIndex((tab) => tab.id === tabId);
  if (index < 0) {
    return null;
  }

  const closedSessionIds = new Set(sessionIds(state.tabs[index]));
  const tabs = state.tabs.filter((tab) => tab.id !== tabId);
  const activeId = state.activeId === tabId
    ? tabs[Math.min(index, tabs.length - 1)]?.id ?? null
    : state.activeId;

  return {
    tabs,
    sessions: state.sessions.filter((session) => !closedSessionIds.has(session.id)),
    activeId,
  };
}

export function closeSession<S extends Session>(
  state: WorkspaceState<S>,
  sessionId: number,
): WorkspaceState<S> | null {
  const tabIndex = state.tabs.findIndex((tab) => sessionIds(tab).includes(sessionId));
  if (tabIndex < 0) {
    return null;
  }

  const tab = state.tabs[tabIndex];
  if (tab.kind === "session") {
    return closeTab(state, tab.id);
  }

  const tileIndex = tab.sessionIds.indexOf(sessionId);
  const remainingSessionIds = tab.sessionIds.filter((id) => id !== sessionId);
  const tabs = [...state.tabs];
  if (remainingSessionIds.length === 1) {
    tabs[tabIndex] = { id: tab.id, kind: "session", sessionId: remainingSessionIds[0] };
  } else {
    tabs[tabIndex] = {
      ...tab,
      sessionIds: remainingSessionIds,
      selectedSessionId: tab.selectedSessionId === sessionId
        ? remainingSessionIds[Math.min(tileIndex, remainingSessionIds.length - 1)]
        : tab.selectedSessionId,
    };
  }

  return {
    tabs,
    sessions: state.sessions.filter((session) => session.id !== sessionId),
    activeId: state.activeId,
  };
}

export function selectSession(
  tabs: readonly Tab[],
  sessionId: number,
): SelectionTransition | null {
  const tab = tabs.find((candidate) => sessionIds(candidate).includes(sessionId));
  if (!tab) {
    return null;
  }

  if (tab.kind === "session") {
    return { tabs: [...tabs], activeId: tab.id };
  }

  const tabIndex = tabs.indexOf(tab);
  const updatedTabs = [...tabs];
  updatedTabs[tabIndex] = { ...tab, selectedSessionId: sessionId };
  return { tabs: updatedTabs, activeId: tab.id };
}

export function tileTab(
  tabs: readonly Tab[],
  sourceTabId: number,
  targetTabId: number,
  nextTabId: number,
): TabTransition | null {
  const source = tabs.find((tab) => tab.id === sourceTabId);
  const target = tabs.find((tab) => tab.id === targetTabId);
  if (!source || source.kind !== "session" || !target || source.id === target.id) {
    return null;
  }

  if (target.kind === "workspace") {
    if (target.sessionIds.length >= maximumWorkspaceSessions) {
      return null;
    }
    const nextTabs = tabs
      .filter((tab) => tab.id !== source.id)
      .map((tab) => tab.id === target.id
        ? { ...target, sessionIds: [...target.sessionIds, source.sessionId], selectedSessionId: source.sessionId }
        : tab);
    return { tabs: nextTabs, activeId: target.id, nextTabId };
  }

  const sourceIndex = tabs.indexOf(source);
  const targetIndex = tabs.indexOf(target);
  const remainingTabs = tabs.filter((tab) => tab.id !== source.id && tab.id !== target.id);
  const workspace: WorkspaceTab = {
    id: nextTabId,
    kind: "workspace",
    sessionIds: [target.sessionId, source.sessionId],
    selectedSessionId: source.sessionId,
  };
  const insertionIndex = targetIndex - (sourceIndex < targetIndex ? 1 : 0);
  remainingTabs.splice(insertionIndex, 0, workspace);
  return { tabs: remainingTabs, activeId: workspace.id, nextTabId: nextTabId + 1 };
}

export function detachSession(
  tabs: readonly Tab[],
  sessionId: number,
  nextTabId: number,
): TabTransition | null {
  const workspaceIndex = tabs.findIndex(
    (tab) => tab.kind === "workspace" && tab.sessionIds.includes(sessionId),
  );
  if (workspaceIndex < 0) {
    return null;
  }

  const workspace = tabs[workspaceIndex] as WorkspaceTab;
  const tileIndex = workspace.sessionIds.indexOf(sessionId);
  const remainingSessionIds = workspace.sessionIds.filter((id) => id !== sessionId);
  const nextTabs = [...tabs];
  if (remainingSessionIds.length === 1) {
    nextTabs[workspaceIndex] = {
      id: workspace.id,
      kind: "session",
      sessionId: remainingSessionIds[0],
    };
  } else {
    nextTabs[workspaceIndex] = {
      ...workspace,
      sessionIds: remainingSessionIds,
      selectedSessionId: workspace.selectedSessionId === sessionId
        ? remainingSessionIds[Math.min(tileIndex, remainingSessionIds.length - 1)]
        : workspace.selectedSessionId,
    };
  }

  const ownTab: Tab = { id: nextTabId, kind: "session", sessionId };
  nextTabs.splice(workspaceIndex + 1, 0, ownTab);
  return { tabs: nextTabs, activeId: ownTab.id, nextTabId: nextTabId + 1 };
}

export function selectTabAt(tabs: readonly Tab[], index: number): number | null {
  return tabs[index]?.id ?? null;
}

export function stepTab(tabs: readonly Tab[], activeId: number | null, delta: number): number | null {
  if (tabs.length === 0) {
    return null;
  }
  const current = tabs.findIndex((tab) => tab.id === activeId);
  const next = (current + delta + tabs.length) % tabs.length;
  return tabs[next].id;
}

export function focusedSessionId(tabs: readonly Tab[], activeId: number | null): number | null {
  const activeTab = tabs.find((tab) => tab.id === activeId);
  if (!activeTab) {
    return null;
  }
  return activeTab.kind === "session" ? activeTab.sessionId : activeTab.selectedSessionId;
}

export function sessionsForCloseTarget<S extends Session>(
  target: CloseTarget,
  tabs: readonly Tab[],
  sessions: readonly S[],
): S[] {
  if (target.kind === "tab") {
    const tab = tabs.find((candidate) => candidate.id === target.id);
    if (!tab) {
      return [];
    }
    const targetSessionIds = new Set(sessionIds(tab));
    return sessions.filter((session) => targetSessionIds.has(session.id));
  }

  const session = sessions.find((candidate) => candidate.id === target.id);
  const ownsSession = tabs.some((tab) => sessionIds(tab).includes(target.id));
  return session && ownsSession ? [session] : [];
}

export function closeConfirmationFor<S extends Session>(
  target: CloseTarget,
  tabs: readonly Tab[],
  sessions: readonly S[],
  labels: ReadonlyMap<number, string>,
): CloseConfirmationRequest | null {
  const targetSessions = sessionsForCloseTarget(target, tabs, sessions);
  if (!targetSessions.some((session) => session.status === "connecting" || session.status === "live")) {
    return null;
  }
  return {
    target,
    sessionIds: targetSessions.map((session) => session.id),
    sessionNames: targetSessions.map((session) => sessionDisplayLabel(labels, session.id)),
    workspace: target.kind === "tab" && tabs.find((tab) => tab.id === target.id)?.kind === "workspace",
  };
}

export function closeTarget<S extends Session>(
  state: WorkspaceState<S>,
  target: CloseTarget,
): WorkspaceState<S> | null {
  return target.kind === "tab" ? closeTab(state, target.id) : closeSession(state, target.id);
}
