import assert from "node:assert/strict";
import test from "node:test";

import {
  sessionDisplayLabel,
  sessionDisplayLabels,
  type Session,
} from "../src/lib/tabs.ts";

function sshSession(
  id: number,
  name: string,
  command: "ssh" | "sftp" = "ssh",
  status: Session["status"] = "live",
  processInstanceId: number | null = id + 100,
) {
  return {
    id,
    connection: { name },
    command,
    status,
    processInstanceId,
  };
}

function localSession(
  id: number,
  status: Session["status"] = "live",
  processInstanceId: number | null = id + 100,
) {
  return {
    id,
    connection: null,
    command: "ssh" as const,
    status,
    processInstanceId,
  };
}

test("unique SSH, SFTP, and local sessions keep distinct base labels", () => {
  const labels = sessionDisplayLabels([
    sshSession(3, "build"),
    sshSession(8, "build", "sftp"),
    localSession(12),
  ]);

  assert.deepEqual([...labels.entries()], [
    [3, "build"],
    [8, "build (sftp)"],
    [12, "Local terminal"],
  ]);
});

test("duplicate labels use ascending logical session IDs regardless of input order", () => {
  const labels = sessionDisplayLabels([
    localSession(30),
    localSession(10),
    localSession(20),
  ]);

  assert.equal(labels.get(10), "Local terminal (1)");
  assert.equal(labels.get(20), "Local terminal (2)");
  assert.equal(labels.get(30), "Local terminal (3)");
});

test("literal numbered names are qualified when they collide with generated labels", () => {
  const labels = sessionDisplayLabels([
    localSession(20),
    sshSession(5, "Local terminal (1)"),
    localSession(8),
  ]);

  assert.equal(labels.get(8), "Local terminal (1) [#8]");
  assert.equal(labels.get(5), "Local terminal (1) [#5]");
  assert.equal(labels.get(20), "Local terminal (2)");
  assert.equal(new Set(labels.values()).size, labels.size);
});

test("qualified labels remain unique when a literal name occupies the first qualifier", () => {
  const labels = sessionDisplayLabels([
    sshSession(1, "a"),
    sshSession(2, "a"),
    sshSession(3, "a (1)"),
    sshSession(4, "a (1) [#1]"),
  ]);

  assert.deepEqual([...labels.entries()], [
    [1, "a (1) [#1.2]"],
    [2, "a (2)"],
    [3, "a (1) [#3]"],
    [4, "a (1) [#1]"],
  ]);
  assert.equal(new Set(labels.values()).size, labels.size);
});

test("chained qualifier collisions use the next bounded qualifier", () => {
  const labels = sessionDisplayLabels([
    sshSession(1, "a"),
    sshSession(2, "a"),
    sshSession(3, "a (1)"),
    sshSession(4, "a (1) [#1]"),
    sshSession(5, "a (1) [#1.2]"),
  ]);

  assert.equal(labels.get(1), "a (1) [#1.3]");
  assert.equal(labels.get(3), "a (1) [#3]");
  assert.equal(new Set(labels.values()).size, labels.size);
});

test("long and closed sessions are included, while missing IDs get safe unique fallbacks", () => {
  const longName = "host-" + "x".repeat(240);
  const labels = sessionDisplayLabels([
    sshSession(4, longName, "ssh", "closed", null),
    localSession(9, "closed", null),
  ]);

  assert.equal(labels.get(4), longName);
  assert.equal(labels.get(9), "Local terminal");
  assert.equal(sessionDisplayLabel(labels, 77), "Session [#77]");
  assert.equal(sessionDisplayLabel(labels, 78), "Session [#78]");
});
