import type { Connection } from "../../bindings/sshbrowse/internal/profile/models";
import { connectionsInFolder, folderParent, isWithinFolder } from "./folderTree.ts";

export type SidebarDragPayload =
  | { kind: "connections"; ids: string[] }
  | { kind: "folder"; path: string };

export function connectionAddress(connection: Pick<Connection, "user" | "host" | "port">): string {
  return `${connection.user ? `${connection.user}@` : ""}${connection.host}${connection.port > 0 ? `:${connection.port}` : ""}`;
}

export function selectedConnections(
  connections: readonly Connection[],
  selectedIds: ReadonlySet<string>,
): Connection[] {
  return connections.filter((connection) => selectedIds.has(connection.id));
}

export function actionConnections(
  connections: readonly Connection[],
  selectedIds: readonly string[],
  connectionId: string,
  orderedIds: readonly string[] = connections.map((connection) => connection.id),
): Connection[] {
  const selected = new Set(selectedIds);
  const targetIds = selected.has(connectionId)
    ? orderedIds.filter((id) => selected.has(id))
    : [connectionId];
  const wanted = new Set(targetIds);
  return connections.filter((connection) => wanted.has(connection.id));
}

export function validDropFolder(
  payload: SidebarDragPayload,
  candidate: string | undefined,
): string | null {
  if (candidate === undefined) {
    return null;
  }
  if (payload.kind === "folder" && (
    isWithinFolder(candidate, payload.path) || folderParent(payload.path) === candidate
  )) {
    return null;
  }
  return candidate;
}

export function connectionsToMove(
  connections: readonly Connection[],
  ids: readonly string[],
  destinationFolder: string,
): Connection[] {
  const wanted = new Set(ids);
  return connections.filter((connection) => wanted.has(connection.id) && connection.folder !== destinationFolder);
}

export function validConnection(
  connections: readonly Connection[],
  id: string,
): Connection | undefined {
  return connections.find((connection) => connection.id === id);
}

export function validFolder(folders: readonly string[], path: string): string | undefined {
  return folders.includes(path) ? path : undefined;
}

export function selectedFolderDestination(folders: readonly string[], selectedFolder: string | null): string {
  return selectedFolder !== null && folders.includes(selectedFolder) ? selectedFolder : "";
}

export function folderContextMenuName(connections: Connection[], folder: string): string {
  return connectionsInFolder(connections, folder).length > 0 ? "saved-folder" : "saved-folder-empty";
}
