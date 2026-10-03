import assert from "node:assert/strict";
import test from "node:test";

import {
  actionConnections,
  connectionAddress,
  connectionsToMove,
  folderContextMenuName,
  selectedFolderDestination,
  selectedConnections,
  validDropFolder,
} from "../src/lib/sidebarActions.ts";

function connection(id: string, folder = "") {
  return { id, name: id, user: id, host: `${id}.example`, port: 22, folder };
}

const connections = [connection("a"), connection("b", "prod"), connection("c", "prod/db")];

test("sidebar action targets preserve source order and selection semantics", () => {
  assert.deepEqual(selectedConnections(connections, new Set(["c", "a"])), [connections[0], connections[2]]);
  assert.deepEqual(actionConnections(connections, ["c", "a"], "c"), [connections[0], connections[2]]);
  assert.deepEqual(actionConnections(connections, ["c", "a"], "b"), [connections[1]]);
});

test("connection addresses are shared by display and clipboard paths", () => {
  assert.equal(connectionAddress(connection("a")), "a@a.example:22");
  assert.equal(connectionAddress({ ...connection("a"), user: "", port: 0 }), "a.example");
});

test("folder drops reject invalid destinations and connection moves skip no-ops", () => {
  const folder = { kind: "folder" as const, path: "prod" };
  assert.equal(validDropFolder(folder, "prod"), null);
  assert.equal(validDropFolder(folder, "prod/db"), null);
  assert.equal(validDropFolder(folder, ""), null);
  assert.equal(validDropFolder(folder, "archive"), "archive");
  assert.deepEqual(connectionsToMove(connections, ["b", "c"], "prod"), [connections[2]]);
});

test("folder menus offer Connect all for direct or nested connections only", () => {
  assert.equal(folderContextMenuName(connections, "prod"), "saved-folder");
  assert.equal(folderContextMenuName(connections, "prod/db"), "saved-folder");
  assert.equal(folderContextMenuName([connection("deep", "prod/db/replicas")], "prod"), "saved-folder");
  assert.equal(folderContextMenuName(connections, "empty"), "saved-folder-empty");
  assert.equal(folderContextMenuName([connection("other", "production")], "prod"), "saved-folder-empty");
  assert.equal(folderContextMenuName([], "prod"), "saved-folder-empty");
});

test("new connections use the selected folder and fall back to the top level", () => {
  const folders = ["prod", "prod/db", "empty"];
  assert.equal(selectedFolderDestination(folders, "prod"), "prod");
  assert.equal(selectedFolderDestination(folders, "prod/db"), "prod/db");
  assert.equal(selectedFolderDestination(folders, "empty"), "empty");
  assert.equal(selectedFolderDestination(folders, null), "");
  assert.equal(selectedFolderDestination(folders, ""), "");
  assert.equal(selectedFolderDestination(folders, "deleted"), "");
});
