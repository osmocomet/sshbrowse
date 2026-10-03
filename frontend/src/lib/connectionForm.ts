import type { Connection } from "../../bindings/sshbrowse/internal/profile/models";

// Show configured options when editing, including older profiles without a saved jump ID.
export function connectionFormSections(connection: Connection) {
  return {
    authentication: !!connection.identityFile,
    routing: !!connection.jumpHost || !!connection.jumpConnectionId,
    forwarding: connection.agentForwarding || connection.x11Forwarding,
    tunnels:
      (connection.localForwards ?? []).length > 0 ||
      (connection.remoteForwards ?? []).length > 0 ||
      (connection.dynamicForwards ?? []).length > 0,
  };
}
