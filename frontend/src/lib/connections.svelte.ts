// Saved connections, shared by the sidebar, the form and the picker.
// Every change goes to Go first and the list is re-read from there.
import * as ConnectionsService from "../../bindings/sshbrowse/internal/app/connections";
import type { Connection } from "../../bindings/sshbrowse/internal/profile/models";

export type ConnectionFormMode = "new" | "edit" | "bookmark" | "duplicate";

export const connections = $state<{ list: Connection[] }>({ list: [] });
export const folders = $state<{ list: string[] }>({ list: [] });

// Go slices arrive as null when empty, hence the fallbacks here and in the form.
export async function loadConnections(): Promise<void> {
  const [loadedConnections, loadedFolders] = await Promise.all([
    ConnectionsService.List(),
    ConnectionsService.ListFolders(),
  ]);
  connections.list = loadedConnections ?? [];
  folders.list = loadedFolders ?? [];
}

export async function saveConnection(connection: Connection): Promise<Connection> {
  const saved = await ConnectionsService.Save(connection);
  await loadConnections();
  return saved;
}

export async function deleteConnection(id: string): Promise<void> {
  await ConnectionsService.Delete(id);
  await loadConnections();
}

// Empty folder means root. A valid new path creates the folder and its ancestors.
export async function moveConnections(list: Connection[], folder: string): Promise<void> {
  await ConnectionsService.Move(list.map((connection) => connection.id), folder);
  await loadConnections();
}

export async function reorderConnections(ids: string[], targetId: string, after: boolean): Promise<void> {
  await ConnectionsService.Reorder(ids, targetId, after);
  await loadConnections();
}

export async function createFolder(path: string): Promise<void> {
  await ConnectionsService.CreateFolder(path);
  await loadConnections();
}

export async function renameFolder(path: string, newPath: string): Promise<void> {
  await ConnectionsService.RenameFolder(path, newPath);
  await loadConnections();
}

export async function deleteFolder(path: string): Promise<void> {
  await ConnectionsService.DeleteFolder(path);
  await loadConnections();
}

// Asks in a native dialog first. Returns false when the user cancelled.
export async function deleteConnections(list: Connection[]): Promise<boolean> {
  const confirmed = await ConnectionsService.ConfirmDelete(list.map((connection) => connection.name));
  if (!confirmed) {
    return false;
  }
  await ConnectionsService.DeleteMany(list.map((connection) => connection.id));
  await loadConnections();
  return true;
}

// "[user@]host[:port]" typed into the picker, as an unsaved connection.
export function parseDestination(text: string): Promise<Connection> {
  return ConnectionsService.Parse(text);
}

export function emptyConnection(): Connection {
  return {
    id: "",
    folder: "",
    name: "",
    host: "",
    user: "",
    port: 0,
    identityFile: "",
    jumpHost: "",
    agentForwarding: false,
    x11Forwarding: false,
    localForwards: [],
    remoteForwards: [],
    dynamicForwards: [],
    provenance: null,
  };
}

export function folderNames(): string[] {
  return folders.list;
}
