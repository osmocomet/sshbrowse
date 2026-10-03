import assert from "node:assert/strict";
import test from "node:test";

import {
  closeConfirmationFor,
  closeSession,
  closeTab,
  detachSession,
  focusedSessionId,
  selectSession,
  stepTab,
  tileTab,
  type WorkspaceState,
} from "../src/lib/workspace.ts";
import { maximumWorkspaceSessions } from "../src/lib/tabs.ts";

function session(id: number) {
  return {
    id,
    connection: null,
    command: "ssh" as const,
    status: "closed" as const,
    processInstanceId: null,
  };
}

function state(tabs: WorkspaceState["tabs"], ids: number[], activeId: number | null): WorkspaceState {
  return { tabs, sessions: ids.map(session), activeId };
}

test("closing a tab removes its sessions and selects the following tab", () => {
  const current = state([
    { id: 10, kind: "session", sessionId: 1 },
    { id: 11, kind: "session", sessionId: 2 },
    { id: 12, kind: "session", sessionId: 3 },
  ], [1, 2, 3], 11);

  assert.deepEqual(closeTab(current, 11), {
    tabs: [
      { id: 10, kind: "session", sessionId: 1 },
      { id: 12, kind: "session", sessionId: 3 },
    ],
    sessions: [session(1), session(3)],
    activeId: 12,
  });
  assert.equal(closeTab(current, 99), null);
});

test("closing a workspace pane converts a one-pane workspace and keeps the adjacent selection", () => {
  const current = state([
    { id: 20, kind: "workspace", sessionIds: [1, 2, 3], selectedSessionId: 2 },
  ], [1, 2, 3], 20);

  assert.deepEqual(closeSession(current, 2), {
    tabs: [{ id: 20, kind: "workspace", sessionIds: [1, 3], selectedSessionId: 3 }],
    sessions: [session(1), session(3)],
    activeId: 20,
  });
  assert.deepEqual(closeSession(closeSession(current, 2)!, 1), {
    tabs: [{ id: 20, kind: "session", sessionId: 3 }],
    sessions: [session(3)],
    activeId: 20,
  });
});

test("tiling and detaching preserve tab order, workspace selection, and id allocation", () => {
  const tabs = [
    { id: 10, kind: "session" as const, sessionId: 1 },
    { id: 11, kind: "session" as const, sessionId: 2 },
    { id: 12, kind: "session" as const, sessionId: 3 },
  ];
  const tiled = tileTab(tabs, 12, 10, 20);
  assert.deepEqual(tiled, {
    tabs: [
      { id: 20, kind: "workspace", sessionIds: [1, 3], selectedSessionId: 3 },
      { id: 11, kind: "session", sessionId: 2 },
    ],
    activeId: 20,
    nextTabId: 21,
  });

  assert.deepEqual(detachSession(tiled!.tabs, 3, tiled!.nextTabId), {
    tabs: [
      { id: 20, kind: "session", sessionId: 1 },
      { id: 21, kind: "session", sessionId: 3 },
      { id: 11, kind: "session", sessionId: 2 },
    ],
    activeId: 21,
    nextTabId: 22,
  });
});

test("tiling a source before its target preserves the adjusted tab order", () => {
  const tabs = [
    { id: 10, kind: "session" as const, sessionId: 1 },
    { id: 11, kind: "session" as const, sessionId: 2 },
    { id: 12, kind: "session" as const, sessionId: 3 },
  ];

  assert.deepEqual(tileTab(tabs, 10, 12, 20), {
    tabs: [
      { id: 11, kind: "session", sessionId: 2 },
      { id: 20, kind: "workspace", sessionIds: [3, 1], selectedSessionId: 1 },
    ],
    activeId: 20,
    nextTabId: 21,
  });
});

test("tiling into an existing workspace appends and selects without allocating a tab id", () => {
  const tabs = [
    { id: 10, kind: "session" as const, sessionId: 1 },
    { id: 11, kind: "workspace" as const, sessionIds: [2, 3], selectedSessionId: 2 },
    { id: 12, kind: "session" as const, sessionId: 4 },
  ];

  assert.deepEqual(tileTab(tabs, 10, 11, 20), {
    tabs: [
      { id: 11, kind: "workspace", sessionIds: [2, 3, 1], selectedSessionId: 1 },
      { id: 12, kind: "session", sessionId: 4 },
    ],
    activeId: 11,
    nextTabId: 20,
  });
});

test("tiling into a full workspace is a no-op", () => {
  const workspace = {
    id: 11,
    kind: "workspace" as const,
    sessionIds: Array.from({ length: maximumWorkspaceSessions }, (_, index) => index + 1),
    selectedSessionId: 1,
  };
  const tabs = [
    workspace,
    { id: 12, kind: "session" as const, sessionId: maximumWorkspaceSessions + 1 },
  ];

  assert.equal(tileTab(tabs, 12, 11, 20), null);
  assert.deepEqual(tabs, [
    workspace,
    { id: 12, kind: "session", sessionId: maximumWorkspaceSessions + 1 },
  ]);
});

test("selection and stepping are pure tab transitions", () => {
  const tabs = [
    { id: 10, kind: "session" as const, sessionId: 1 },
    { id: 20, kind: "workspace" as const, sessionIds: [2, 3], selectedSessionId: 2 },
  ];
  assert.deepEqual(selectSession(tabs, 3), {
    tabs: [
      { id: 10, kind: "session", sessionId: 1 },
      { id: 20, kind: "workspace", sessionIds: [2, 3], selectedSessionId: 3 },
    ],
    activeId: 20,
  });
  assert.equal(stepTab(tabs, 10, 1), 20);
  assert.equal(stepTab(tabs, 10, -1), 20);
  assert.equal(focusedSessionId(tabs, 20), 2);
});

test("close confirmation is required when any target session is live and captures the full close target", () => {
  const sessions = [
    { ...session(1), status: "live" as const },
    { ...session(2), status: "closed" as const },
  ];
  const tabs = [{ id: 10, kind: "workspace" as const, sessionIds: [1, 2], selectedSessionId: 1 }];
  const labels = new Map([[1, "one"], [2, "two"]]);

  assert.deepEqual(closeConfirmationFor({ kind: "tab", id: 10 }, tabs, sessions, labels), {
    target: { kind: "tab", id: 10 },
    sessionIds: [1, 2],
    sessionNames: ["one", "two"],
    workspace: true,
  });
  assert.equal(closeConfirmationFor({ kind: "tab", id: 10 }, tabs, [session(2)], labels), null);
});
