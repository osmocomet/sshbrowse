import assert from "node:assert/strict";
import test from "node:test";

import {
  createLiveBroadcastWriter,
  pasteClipboardForBroadcastGeneration,
  liveBroadcastPasteError,
  liveBroadcastPasteGenerationError,
  liveBroadcastSnapshotInvalidationReason,
  liveBroadcastStartError,
} from "../src/lib/liveBroadcast.ts";
import { terminalPastePrompt } from "../src/lib/terminalInput.ts";
import type { BroadcastSnapshot } from "../src/lib/routing.ts";
import type { SessionStatus } from "../src/lib/tabs.ts";

function target(logicalSessionId: number, processInstanceId: number): BroadcastSnapshot {
  return { logicalSessionId, processInstanceId, label: `session ${logicalSessionId}` };
}

function liveSession(id: number, processInstanceId: number, status: SessionStatus = "live") {
  return { id, processInstanceId, status };
}

async function settle() {
  await new Promise<void>((resolve) => setImmediate(resolve));
}

test("live broadcast requires two live targets and includes focus", () => {
  const one = [target(1, 101)];
  const two = [...one, target(2, 102)];

  assert.match(liveBroadcastStartError(one, 1) ?? "", /at least two/);
  assert.match(liveBroadcastStartError(two, 3) ?? "", /focused terminal/);
  assert.equal(liveBroadcastStartError(two, 1), null);
});

test("snapshot validity preserves process-instance identity", () => {
  const snapshot = [target(1, 101), target(2, 102)];
  const sessions = [liveSession(1, 101), liveSession(2, 102)];

  assert.equal(liveBroadcastSnapshotInvalidationReason(snapshot, sessions, 1), null);
  assert.match(
    liveBroadcastSnapshotInvalidationReason(snapshot, [liveSession(1, 201), sessions[1]], 1) ?? "",
    /changed process instances/,
  );
  assert.match(
    liveBroadcastSnapshotInvalidationReason(snapshot, [liveSession(1, 101, "closed"), sessions[1]], 1) ?? "",
    /no longer live/,
  );
  assert.match(liveBroadcastSnapshotInvalidationReason(snapshot, [sessions[0]], 1) ?? "", /disappeared/);
});

test("focus within the snapshot is allowed but focus outside stops it", () => {
  const snapshot = [target(1, 101), target(2, 102)];
  const sessions = [liveSession(1, 101), liveSession(2, 102)];

  assert.equal(liveBroadcastSnapshotInvalidationReason(snapshot, sessions, 2), null);
  assert.match(
    liveBroadcastSnapshotInvalidationReason(snapshot, [...sessions, liveSession(3, 103)], 3) ?? "",
    /focus moved outside/,
  );
});

test("ordered writer preserves raw control and escape bytes for every target", async () => {
  const snapshot = [target(1, 101), target(2, 102)];
  const rawInput = "\x03\x1b[1;5C\x7f\t\r";
  const writes: Array<{ processInstanceId: number; input: string }> = [];
  const writer = createLiveBroadcastWriter(snapshot, async (processInstanceId, input) => {
    writes.push({ processInstanceId, input });
  }, () => assert.fail("writer should not stop"));

  writer.enqueue("first");
  writer.enqueue(rawInput);
  writer.enqueue("last");
  await settle();

  assert.deepEqual(writes, [
    { processInstanceId: 101, input: "first" },
    { processInstanceId: 102, input: "first" },
    { processInstanceId: 101, input: rawInput },
    { processInstanceId: 102, input: rawInput },
    { processInstanceId: 101, input: "last" },
    { processInstanceId: 102, input: "last" },
  ]);
});

test("source-only replies stay ordered with mirrored input", async () => {
  const writes: Array<{ processInstanceId: number; input: string }> = [];
  const writer = createLiveBroadcastWriter(
    [target(1, 101), target(2, 102)],
    async (processInstanceId, input) => writes.push({ processInstanceId, input }),
    () => assert.fail("writer should not stop"),
  );

  writer.enqueue("typed");
  writer.enqueueTo(101, "reply");
  writer.enqueue("after");
  await settle();

  assert.deepEqual(writes, [
    { processInstanceId: 101, input: "typed" },
    { processInstanceId: 102, input: "typed" },
    { processInstanceId: 101, input: "reply" },
    { processInstanceId: 101, input: "after" },
    { processInstanceId: 102, input: "after" },
  ]);
});

test("a write failure stops the writer and names its target", async () => {
  const writes: Array<{ processInstanceId: number; input: string }> = [];
  const stops: string[] = [];
  const writer = createLiveBroadcastWriter(
    [target(1, 101), target(2, 102)],
    async (processInstanceId, input) => {
      writes.push({ processInstanceId, input });
      if (processInstanceId === 102) throw new Error("closed");
    },
    (reason) => stops.push(reason),
  );

  writer.enqueue("one");
  writer.enqueue("two");
  await settle();

  assert.equal(writer.isStopped(), true);
  assert.match(stops[0] ?? "", /session 2/);
  assert.match(stops[0] ?? "", /diverged/);
  assert.deepEqual(writes, [
    { processInstanceId: 101, input: "one" },
    { processInstanceId: 102, input: "one" },
  ]);
});

test("a hung target write times out and stops broadcasting", async () => {
  const snapshot = [target(1, 101), target(2, 102)];
  let reportStop!: (reason: string) => void;
  const stopped = new Promise<string>((resolve) => { reportStop = resolve; });
  const writer = createLiveBroadcastWriter(
    snapshot,
    async (processInstanceId) => {
      if (processInstanceId === 102) {
        await new Promise<void>(() => {});
      }
    },
    reportStop,
    { writeTimeoutMs: 5 },
  );

  writer.enqueue("input");
  const reason = await stopped;

  assert.equal(writer.isStopped(), true);
  assert.match(reason, /session 2 failed: target write timed out/);
  assert.match(reason, /diverged/);
});

test("manual stop clears queued input without reporting an error", async () => {
  let releaseIssuedWrite: (() => void) | undefined;
  const writes: string[] = [];
  const reportedStops: string[] = [];
  const writer = createLiveBroadcastWriter(
    [target(1, 101)],
    async (_processInstanceId, input) => {
      writes.push(input);
      if (input === "issued") {
        await new Promise<void>((resolve) => { releaseIssuedWrite = resolve; });
      }
    },
    (reason) => reportedStops.push(reason),
  );

  writer.enqueue("issued");
  writer.enqueue("queued");
  while (releaseIssuedWrite === undefined) {
    await new Promise<void>((resolve) => setImmediate(resolve));
  }
  writer.stop();

  assert.equal(writer.isStopped(), true);
  assert.equal(writer.pendingBytes(), 0);
  assert.equal(writer.enqueue("after stop"), false);
  releaseIssuedWrite();
  await settle();

  assert.deepEqual(writes, ["issued"]);
  assert.deepEqual(reportedStops, []);
});

test("queue overflow stops broadcasting and bounds pending bytes", async () => {
  const stops: string[] = [];
  let releaseFirstWrite: (() => void) | undefined;
  const writer = createLiveBroadcastWriter(
    [target(1, 101), target(2, 102)],
    async () => new Promise<void>((resolve) => { releaseFirstWrite = resolve; }),
    (reason) => stops.push(reason),
    { maximumQueuedBytes: 4, writeTimeoutMs: 100 },
  );

  writer.enqueue("abc");
  assert.equal(writer.enqueue("de"), false);
  assert.equal(writer.isStopped(), true);
  assert.equal(writer.pendingBytes(), 0);
  assert.match(stops[0] ?? "", /queue exceeded/);
  releaseFirstWrite?.();
  await settle();
});

test("broadcast paste confirmation uses the recipient count", () => {
  assert.equal(terminalPastePrompt("focused", 14, 5), "Paste 14 lines into 5 sessions?");
  assert.equal(terminalPastePrompt("focused", 1, 5), "Paste 1 line into 5 sessions?");
});

test("stale broadcast paste is rejected after a process change", () => {
  const snapshot = [target(1, 101), target(2, 102)];
  const sessions = [liveSession(1, 201), liveSession(2, 102)];

  assert.match(
    liveBroadcastPasteError(snapshot, sessions, 1, 1, 101, 2) ?? "",
    /changed process instances/,
  );
  assert.equal(liveBroadcastPasteError(snapshot, [liveSession(1, 101), liveSession(2, 102)], 1, 1, 101, 2), null);
});

test("clipboard paste keeps the broadcast generation captured before the read", () => {
  // A -> B changes twice: stopping A and starting B. Equal recipient counts do not make A's paste valid for B.
  const broadcastB = [target(1, 101), target(3, 103)];
  assert.equal(
    liveBroadcastPasteError(broadcastB, [liveSession(1, 101), liveSession(3, 103)], 1, 1, 101, 2),
    null,
  );
  assert.match(liveBroadcastPasteGenerationError(4, 6) ?? "", /broadcast changed/);

  // A paste started while broadcasting is off must also be rejected if a broadcast starts during the clipboard read.
  assert.match(liveBroadcastPasteGenerationError(8, 9) ?? "", /broadcast changed/);
  assert.equal(liveBroadcastPasteGenerationError(9, 9), null);
});

test("deferred clipboard reads cannot paste into a newer broadcast", async () => {
  async function checkGenerationChange(
    startGeneration: number,
    startRecipientCount: number | null,
    nextGeneration: number,
    nextRecipientCount: number | null,
  ) {
    let state = { generation: startGeneration, recipientCount: startRecipientCount };
    let resolveClipboard!: (text: string) => void;
    const pastes: string[] = [];
    const rejected: Array<[number, number | null, number, number | null]> = [];
    const pendingPaste = pasteClipboardForBroadcastGeneration(
      () => new Promise<string>((resolve) => { resolveClipboard = resolve; }),
      () => state,
      (text) => pastes.push(text),
      (captured, current) => rejected.push([
        captured.generation,
        captured.recipientCount,
        current.generation,
        current.recipientCount,
      ]),
    );

    state = { generation: nextGeneration, recipientCount: nextRecipientCount };
    resolveClipboard("clipboard contents");
    await pendingPaste;

    assert.deepEqual(pastes, []);
    assert.deepEqual(rejected, [[startGeneration, startRecipientCount, nextGeneration, nextRecipientCount]]);
  }

  // Broadcast A stops and B starts while the read is pending.
  await checkGenerationChange(4, 2, 6, 2);
  // Broadcasting starts while a non-broadcast clipboard read is pending.
  await checkGenerationChange(8, null, 9, 2);
});
