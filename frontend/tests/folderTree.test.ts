import assert from "node:assert/strict";
import test from "node:test";

import {
  buildFolderTree,
  connectionsInFolder,
  folderCreationParent,
  visibleConnectionIds,
  type FolderTreeConnection,
} from "../src/lib/folderTree.ts";

interface TestConnection extends FolderTreeConnection {
  name: string;
}

test("buildFolderTree keeps folder and connection order including empty folders", () => {
  const connections: TestConnection[] = [
    { id: "root", folder: "", name: "Root" },
    { id: "prod", folder: "Team/Production", name: "Production" },
    { id: "team", folder: "Team", name: "Team" },
  ];
  const tree = buildFolderTree(["Team", "Team/Production", "Archive"], connections);

  assert.deepEqual(tree.connections.map((connection) => connection.id), ["root"]);
  assert.deepEqual(tree.folders.map((folder) => folder.path), ["Team", "Archive"]);
  assert.deepEqual(tree.folders[0].connections.map((connection) => connection.id), ["team"]);
  assert.deepEqual(tree.folders[0].children.map((folder) => folder.path), ["Team/Production"]);
  assert.deepEqual(tree.folders[1].connections, []);
});

test("buildFolderTree supplies missing ancestors without duplicating persisted folders", () => {
  const tree = buildFolderTree(["Customers/Acme"], [
    { id: "deep", folder: "Customers/Acme/Production", name: "Deep" },
  ]);

  assert.equal(tree.folders[0].path, "Customers");
  assert.equal(tree.folders[0].children[0].path, "Customers/Acme");
  assert.equal(tree.folders[0].children[0].children[0].path, "Customers/Acme/Production");
});

test("visibleConnectionIds follows the recursive visible order", () => {
  const tree = buildFolderTree(
    ["Team", "Team/Production", "Archive"],
    [
      { id: "archive", folder: "Archive", name: "Archive" },
      { id: "prod", folder: "Team/Production", name: "Production" },
      { id: "root", folder: "", name: "Root" },
      { id: "team", folder: "Team", name: "Team" },
    ],
  );

  assert.deepEqual(visibleConnectionIds(tree, {}), ["root", "team", "prod", "archive"]);
  assert.deepEqual(visibleConnectionIds(tree, { Team: true }), ["root", "archive"]);
  assert.deepEqual(visibleConnectionIds(tree, { "Team/Production": true }), ["root", "team", "archive"]);
});

test("connectionsInFolder includes direct and nested descendants", () => {
  const connections: TestConnection[] = [
    { id: "root", folder: "", name: "Root" },
    { id: "team", folder: "Team", name: "Team" },
    { id: "prod", folder: "Team/Production", name: "Production" },
    { id: "teamwork", folder: "Teamwork", name: "Teamwork" },
    { id: "other", folder: "Other", name: "Other" },
  ];

  assert.deepEqual(connectionsInFolder(connections, "Team").map((connection) => connection.id), ["team", "prod"]);
  assert.deepEqual(connectionsInFolder(connections, "Team/Production").map((connection) => connection.id), ["prod"]);
  assert.deepEqual(connectionsInFolder(connections, "Archive"), []);
});

test("folder creation resolves the clicked background, connection, or folder target", () => {
  const connections = [
    { id: "top", folder: "" },
    { id: "nested", folder: "Team/Production" },
  ];
  const folders = ["Team", "Team/Production", "Empty"];

  assert.equal(folderCreationParent({ kind: "background" }, [], []), "");
  assert.equal(folderCreationParent({ kind: "connection", id: "top" }, connections, folders), "");
  assert.equal(folderCreationParent({ kind: "connection", id: "nested" }, connections, folders), "Team/Production");
  assert.equal(folderCreationParent({ kind: "folder", path: "Team/Production" }, connections, folders), "Team/Production");
  assert.equal(folderCreationParent({ kind: "connection", id: "missing" }, connections, folders), null);
  assert.equal(folderCreationParent({ kind: "folder", path: "Missing" }, connections, folders), null);
});
