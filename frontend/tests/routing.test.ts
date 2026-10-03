import assert from "node:assert/strict";
import test from "node:test";

import {
  broadcastDeliverySkippedError,
  broadcastDeliveryUnknownError,
  broadcastCommandError,
  broadcastCommandLineCount,
  broadcastRecipients,
  createBroadcastSender,
  maximumBroadcastCommandBytes,
  normalizeBroadcastCommand,
  placeNewSession,
  snapshotBroadcastRecipients,
} from "../src/lib/routing.ts";
import { sessionDisplayLabels } from "../src/lib/tabs.ts";

function session(id: number, name: string, status: "connecting" | "live" | "closed", processInstanceId: number | null) {
  return {
    id,
    connection: { name },
    command: "ssh" as const,
    status,
    processInstanceId,
  };
}

function localSession(id: number, status: "connecting" | "live" | "closed", processInstanceId: number | null) {
  return {
    id,
    connection: null,
    command: "ssh" as const,
    status,
    processInstanceId,
  };
}

test("tiling fills the active workspace and starts another after nine", () => {
  let placement = placeNewSession([], null, 1, true, 10);
  assert.deepEqual(placement.tabs, [{ id: 10, kind: "session", sessionId: 1 }]);

  placement = placeNewSession(placement.tabs, placement.activeId, 2, true, placement.nextTabId);
  assert.deepEqual(placement.tabs[0], {
    id: 10,
    kind: "workspace",
    sessionIds: [1, 2],
    selectedSessionId: 2,
  });
  for (let sessionId = 3; sessionId <= 9; sessionId += 1) {
    placement = placeNewSession(placement.tabs, placement.activeId, sessionId, true, placement.nextTabId);
  }
  assert.equal(placement.tabs.length, 1);
  assert.deepEqual(placement.tabs[0], {
    id: 10,
    kind: "workspace",
    sessionIds: [1, 2, 3, 4, 5, 6, 7, 8, 9],
    selectedSessionId: 9,
  });

  placement = placeNewSession(placement.tabs, placement.activeId, 10, true, placement.nextTabId);
  assert.equal(placement.tabs.length, 2);
  assert.deepEqual(placement.tabs[1], { id: 11, kind: "session", sessionId: 10 });

  placement = placeNewSession(placement.tabs, placement.activeId, 11, true, placement.nextTabId);
  assert.deepEqual(placement.tabs[1], {
    id: 11,
    kind: "workspace",
    sessionIds: [10, 11],
    selectedSessionId: 11,
  });
});

test("disabled tiling always creates an ordinary tab", () => {
  const placement = placeNewSession(
    [{ id: 1, kind: "workspace", sessionIds: [1, 2], selectedSessionId: 2 }],
    1,
    3,
    false,
    2,
  );
  assert.deepEqual(placement.tabs[1], { id: 2, kind: "session", sessionId: 3 });
});

test("recipient routing follows scope, labels duplicates, and disables unavailable sessions", () => {
  const sessions = [
    session(1, "web", "live", 101),
    session(2, "web", "closed", null),
    session(3, "db", "connecting", 103),
  ];
  const tabs = [
    { id: 10, kind: "workspace" as const, sessionIds: [1, 2], selectedSessionId: 1 },
    { id: 11, kind: "session" as const, sessionId: 3 },
  ];

  const current = broadcastRecipients(sessions, tabs, 10, "current");
  assert.deepEqual(current.map(({ label, available }) => ({ label, available })), [
    { label: "web (1)", available: true },
    { label: "web (2)", available: false },
  ]);
  assert.deepEqual(broadcastRecipients(sessions, tabs, 11, "current").map((recipient) => recipient.label), ["db"]);
  assert.equal(broadcastRecipients(sessions, tabs, null, "current").length, 0);
  assert.equal(broadcastRecipients(sessions, tabs, 10, "all").length, 3);
  assert.deepEqual(snapshotBroadcastRecipients(current, new Set([1])), []);
});

test("recipient labels stay global across scopes and tab order", () => {
  const sessions = [
    localSession(30, "live", 130),
    localSession(10, "live", 110),
    localSession(20, "live", 120),
  ];
  const tabs = [
    { id: 11, kind: "session" as const, sessionId: 30 },
    { id: 12, kind: "workspace" as const, sessionIds: [20, 10], selectedSessionId: 10 },
  ];
  const labels = sessionDisplayLabels(sessions);

  assert.deepEqual(
    broadcastRecipients(sessions, tabs, 11, "current", labels).map((recipient) => recipient.label),
    ["Local terminal (3)"],
  );
  assert.deepEqual(
    broadcastRecipients(sessions, tabs, 12, "current", labels).map((recipient) => recipient.label),
    ["Local terminal (2)", "Local terminal (1)"],
  );
  assert.deepEqual(
    broadcastRecipients(sessions, tabs, 11, "all", labels).map((recipient) => recipient.label),
    ["Local terminal (3)", "Local terminal (2)", "Local terminal (1)"],
  );
  assert.deepEqual(
    snapshotBroadcastRecipients(
      broadcastRecipients(sessions, tabs, 11, "all", labels),
      new Set([20]),
    ).map((recipient) => recipient.label),
    ["Local terminal (3)", "Local terminal (1)"],
  );
});

test("a send uses its process-instance snapshot and reports partial failures", async () => {
  const sessions = [session(1, "web", "live", 101), session(2, "db", "live", 102)];
  const tabs = [{ id: 10, kind: "workspace" as const, sessionIds: [1, 2], selectedSessionId: 1 }];
  const snapshot = snapshotBroadcastRecipients(broadcastRecipients(sessions, tabs, 10, "current"), new Set());
  sessions[0].processInstanceId = 201;
  const writes: Array<{ id: number; data: string }> = [];
  const sender = createBroadcastSender(async (id, data) => {
    writes.push({ id, data });
    if (id === 102) {
      throw new Error("session closed");
    }
  });
  const deliveries = await sender.send(snapshot, "uptime");

  assert.deepEqual(writes, [{ id: 101, data: "uptime\r" }, { id: 102, data: "uptime\r" }]);
  assert.equal(deliveries[0].error, null);
  assert.match(deliveries[1].error ?? "", /session closed/);
});

test("delivery results retain labels captured before duplicate compaction", async () => {
  const initialSessions = [
    session(1, "web", "live", 101),
    session(2, "web", "live", 102),
  ];
  const tabs = [
    { id: 10, kind: "workspace" as const, sessionIds: [1, 2], selectedSessionId: 2 },
  ];
  const initialLabels = sessionDisplayLabels(initialSessions);
  const snapshot = snapshotBroadcastRecipients(
    broadcastRecipients(initialSessions, tabs, 10, "current", initialLabels),
    new Set([1]),
  );
  assert.equal(snapshot[0].label, "web (2)");

  const currentSessions = [initialSessions[1]];
  const currentLabels = sessionDisplayLabels(currentSessions);
  assert.equal(broadcastRecipients(currentSessions, tabs, 10, "current", currentLabels)[0].label, "web");

  const sender = createBroadcastSender(async () => {});
  const deliveries = await sender.send(snapshot, "uptime");
  assert.equal(deliveries[0].label, "web (2)");
});

test("broadcast writes have bounded concurrency", async () => {
  const recipients = Array.from({ length: 12 }, (_, index) => ({
    logicalSessionId: index,
    processInstanceId: 100 + index,
    label: `session ${index}`,
  }));
  let activeWrites = 0;
  let peakWrites = 0;
  const releases: Array<() => void> = [];
  const sender = createBroadcastSender(async () => {
    activeWrites++;
    peakWrites = Math.max(peakWrites, activeWrites);
    await new Promise<void>((resolve) => releases.push(resolve));
    activeWrites--;
  });
  const sending = sender.send(recipients, "date");
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(peakWrites, 4);
  while (releases.length > 0) {
    releases.shift()?.();
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  await sending;
});

test("a deadline reports issued writes as unknown and queued recipients as skipped", async (context) => {
  context.mock.timers.enable({ apis: ["setTimeout"] });
  const recipients = Array.from({ length: 6 }, (_, index) => ({
    logicalSessionId: index,
    processInstanceId: 200 + index,
    label: `session ${index}`,
  }));
  const releases: Array<() => void> = [];
  const sender = createBroadcastSender(
    async () => new Promise<void>((resolve) => releases.push(resolve)),
    { deadlineMs: 10 },
  );

  const sending = sender.send(recipients, "date");
  context.mock.timers.tick(10);
  const deliveries = await sending;

  assert.equal(releases.length, 4);
  assert.deepEqual(deliveries.map((delivery) => delivery.error), [
    broadcastDeliveryUnknownError,
    broadcastDeliveryUnknownError,
    broadcastDeliveryUnknownError,
    broadcastDeliveryUnknownError,
    broadcastDeliverySkippedError,
    broadcastDeliverySkippedError,
  ]);
  assert.equal(sender.isBusy(), true);

  releases.forEach((release) => release());
  await new Promise<void>((resolve) => setImmediate(resolve));
  assert.equal(sender.isBusy(), false);
});

test("a new batch cannot add writes while an earlier batch is unresolved", async (context) => {
  context.mock.timers.enable({ apis: ["setTimeout"] });
  const recipients = [{ logicalSessionId: 1, processInstanceId: 301, label: "session" }];
  let writeCount = 0;
  let releaseWrite: (() => void) | undefined;
  const sender = createBroadcastSender(
    async () => {
      writeCount++;
      if (writeCount === 1) {
        await new Promise<void>((resolve) => { releaseWrite = resolve; });
      }
    },
    { deadlineMs: 10 },
  );

  const firstSend = sender.send(recipients, "first");
  context.mock.timers.tick(10);
  const firstDeliveries = await firstSend;
  assert.equal(firstDeliveries[0].error, broadcastDeliveryUnknownError);
  await assert.rejects(sender.send(recipients, "second"), /still finishing/);
  assert.equal(writeCount, 1);
  assert.equal(sender.isBusy(), true);

  releaseWrite?.();
  await new Promise<void>((resolve) => setImmediate(resolve));
  assert.equal(sender.isBusy(), false);

  const finalDeliveries = await sender.send(recipients, "third");
  assert.equal(finalDeliveries[0].error, null);
  assert.equal(writeCount, 2);
});

test("normalizes command blocks to terminal Enter semantics", () => {
  assert.equal(normalizeBroadcastCommand("echo one"), "echo one\r");
  assert.equal(normalizeBroadcastCommand("echo one\necho two"), "echo one\recho two\r");
  assert.equal(normalizeBroadcastCommand("echo one\r\necho two"), "echo one\recho two\r");
  assert.equal(normalizeBroadcastCommand("echo one\recho two"), "echo one\recho two\r");
  assert.equal(normalizeBroadcastCommand("echo one\necho two\n"), "echo one\recho two\r");
  assert.equal(normalizeBroadcastCommand("echo one\n\necho three"), "echo one\r\recho three\r");
  assert.equal(normalizeBroadcastCommand("echo one\n\n"), "echo one\r");
  assert.equal(broadcastCommandLineCount("echo one\n\necho three"), 3);
});

test("command validation accepts multiline blocks but rejects staged controls and oversized UTF-8 input", () => {
  assert.equal(broadcastCommandError("one\ntwo"), null);
  assert.match(broadcastCommandError("   \t") ?? "", /Enter a command/);
  assert.match(broadcastCommandError("one\ttwo") ?? "", /control characters/);
  assert.match(broadcastCommandError("printf '\u001b'") ?? "", /control characters/);
  assert.match(broadcastCommandError("é".repeat(maximumBroadcastCommandBytes)) ?? "", /16 KB/);
  assert.equal(broadcastCommandError("echo ready"), null);
});

test("a multiline command sender writes one terminal Enter per command block", async () => {
  const writes: Array<{ id: number; data: string }> = [];
  const sender = createBroadcastSender(async (id, data) => {
    writes.push({ id, data });
  });
  await sender.send([{ logicalSessionId: 1, processInstanceId: 101, label: "session" }], "echo one\necho two\n");
  assert.deepEqual(writes, [{ id: 101, data: "echo one\recho two\r" }]);
});
