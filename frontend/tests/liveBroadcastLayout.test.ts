import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";
import ts from "typescript";
import { createLiveBroadcastWriter, liveBroadcastLayoutInvalidationReason, liveBroadcastSnapshotInvalidationReason } from "../src/lib/liveBroadcast.ts";
import { detachSession, focusedSessionId, selectSession, tileTab } from "../src/lib/workspace.ts";
import { isTerminalTargetCurrent } from "../src/lib/terminalInput.ts";
import type { Tab } from "../src/lib/tabs.ts";

const tabs: Tab[] = [
  { id: 10, kind: "workspace", sessionIds: [1, 2], selectedSessionId: 1 },
  { id: 20, kind: "session", sessionId: 3 },
];
const snapshot = [1, 2, 3].map((id) => ({ logicalSessionId: id, processInstanceId: id + 100, label: `session ${id}` }));

// Exercise the actual App actions so removing their safety guard fails the test.
const source = readFileSync(new URL("../src/App.svelte", import.meta.url), "utf8");
const script = source.slice(source.indexOf(">", source.indexOf("<script")) + 1, source.indexOf("</script>"));
const ast = ts.createSourceFile("App.ts", script, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
const names = ["applyTabLayout", "stopLiveBroadcast", "tileTab", "detachSession", "routeTerminalInput"];
const actions = ast.statements.filter((node) => ts.isFunctionDeclaration(node) && node.name && names.includes(node.name.text));
assert.equal(actions.length, names.length);
const appActions = ts.transpile(actions.map((node) => node.getText(ast)).join("\n"), { target: ts.ScriptTarget.ES2022 });

for (const action of ["detachSession(1)", "tileTab(20, 10)"]) {
  test(`${action} stops before layout assignment, discards queued input and routes subsequent input locally`, async () => {
    const writes: Array<[number, string]> = [];
    const write = async (id: number, input: string) => { writes.push([id, input]); };
    const writer = createLiveBroadcastWriter(snapshot, write, () => {});
    const state = {
      activeId: 10, nextTabId: 30,
      sessions: snapshot.map((target) => ({ id: target.logicalSessionId, processInstanceId: target.processInstanceId, status: "live" })),
      liveBroadcast: { snapshot } as { snapshot: typeof snapshot } | null,
      liveBroadcastWriter: writer, liveBroadcastGeneration: 1, liveBroadcastError: "",
      liveBroadcastLayoutInvalidationReason, liveBroadcastSnapshotInvalidationReason,
      detachSessionState: detachSession, tileTabState: tileTab,
      focusedSessionId, isTerminalTargetCurrent, Sessions: { Write: write }, console,
    };
    let currentTabs = tabs;
    Object.defineProperty(state, "tabs", {
      get: () => currentTabs,
      set: (next: Tab[]) => {
        assert.equal(writer.isStopped(), true, "writer must stop before layout assignment");
        assert.equal(writer.pendingBytes(), 0);
        currentTabs = next;
      },
    });
    const context = vm.createContext(state);
    vm.runInContext(appActions, context);
    writer.enqueue("discard");
    vm.runInContext(action, context);
    assert.equal(state.liveBroadcast, null);
    assert.equal(state.liveBroadcastGeneration, 2);
    assert.match(state.liveBroadcastError, /layout changed/);
    const sessionId = focusedSessionId(currentTabs, state.activeId)!;
    context.input = { sessionId, processInstanceId: sessionId + 100, input: "after", mirrorToBroadcast: true };
    vm.runInContext("routeTerminalInput(input)", context);
    await new Promise<void>((resolve) => setImmediate(resolve));
    assert.deepEqual(writes, [[sessionId + 100, "after"]]);
  });
}

test("tile selection and All tabs navigation preserve the broadcast context", () => {
  const selected = selectSession(tabs, 2)!;
  assert.equal(liveBroadcastLayoutInvalidationReason(snapshot, tabs, selected.tabs), null);
  assert.equal(liveBroadcastLayoutInvalidationReason(snapshot, tabs, [...tabs].reverse()), null);
  const sessions = snapshot.map((target) => ({ id: target.logicalSessionId, processInstanceId: target.processInstanceId, status: "live" as const }));
  for (const activeId of [selected.activeId, 20, 10]) {
    assert.equal(liveBroadcastSnapshotInvalidationReason(snapshot, sessions, focusedSessionId(selected.tabs, activeId)), null);
  }
});
