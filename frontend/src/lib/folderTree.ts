// Folder paths are slash-separated, "" being the top level. The Go store owns
// validation; these helpers only split and join what it has already accepted.

export interface FolderTreeConnection {
  id: string;
  folder: string;
}

export interface FolderNode<T extends FolderTreeConnection> {
  path: string;
  name: string;
  connections: T[];
  children: FolderNode<T>[];
}

export interface FolderTree<T extends FolderTreeConnection> {
  connections: T[];
  folders: FolderNode<T>[];
}

export type FolderCreationContext =
  | { kind: "background" }
  | { kind: "connection"; id: string }
  | { kind: "folder"; path: string };

export type FolderAction = { kind: "create"; parent: string } | { kind: "rename"; path: string };

export function folderParent(path: string): string {
  const separator = path.lastIndexOf("/");
  return separator < 0 ? "" : path.slice(0, separator);
}

export function folderBaseName(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1);
}

export function joinFolderPath(parent: string, name: string): string {
  return parent === "" ? name : `${parent}/${name}`;
}

// Resolve only the existing context target. The dialog and Go store continue
// to own name validation and persistence.
export function folderCreationParent<T extends FolderTreeConnection>(
  context: FolderCreationContext,
  connections: T[],
  folders: string[],
): string | null {
  switch (context.kind) {
    case "background":
      return "";
    case "connection":
      return connections.find((connection) => connection.id === context.id)?.folder ?? null;
    case "folder":
      return folders.includes(context.path) ? context.path : null;
  }
}

// True for the folder itself and everything below it.
export function isWithinFolder(path: string, ancestor: string): boolean {
  return path === ancestor || path.startsWith(`${ancestor}/`);
}

export function connectionsInFolder<T extends FolderTreeConnection>(connections: T[], folder: string): T[] {
  return connections.filter((connection) => isWithinFolder(connection.folder, folder));
}

// Folders keep the order of folderPaths; connections keep their own order.
// A connection whose folder is missing from folderPaths still gets a node.
export function buildFolderTree<T extends FolderTreeConnection>(folderPaths: string[], connections: T[]): FolderTree<T> {
  const tree: FolderTree<T> = { connections: [], folders: [] };
  const nodes = new Map<string, FolderNode<T>>();

  function ensureFolder(path: string): FolderNode<T> | null {
    if (path === "") {
      return null;
    }
    const existing = nodes.get(path);
    if (existing) {
      return existing;
    }
    const node: FolderNode<T> = { path, name: folderBaseName(path), connections: [], children: [] };
    nodes.set(path, node);
    const parent = ensureFolder(folderParent(path));
    (parent?.children ?? tree.folders).push(node);
    return node;
  }

  for (const path of folderPaths) {
    ensureFolder(path);
  }
  for (const connection of connections) {
    const folder = ensureFolder(connection.folder);
    (folder?.connections ?? tree.connections).push(connection);
  }
  return tree;
}

// Connection ids in display order, skipping collapsed folders and their subtrees.
export function visibleConnectionIds<T extends FolderTreeConnection>(
  tree: FolderTree<T>,
  collapsed: Record<string, boolean>,
): string[] {
  const ids = tree.connections.map((connection) => connection.id);
  function appendFolder(folder: FolderNode<T>) {
    if (collapsed[folder.path]) {
      return;
    }
    ids.push(...folder.connections.map((connection) => connection.id));
    folder.children.forEach(appendFolder);
  }
  tree.folders.forEach(appendFolder);
  return ids;
}
