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

export function forwardingSummary(connection: Pick<Connection, "agentForwarding" | "x11Forwarding">): string {
  if (connection.agentForwarding && connection.x11Forwarding) return "SSH agent and X11 enabled";
  if (connection.agentForwarding) return "SSH agent enabled";
  if (connection.x11Forwarding) return "X11 enabled";
  return "Use your local SSH agent or X11 display remotely";
}

export function portForwardingSummary(...forwardingText: string[]): string {
  let tunnelCount = 0;
  for (const text of forwardingText) {
    tunnelCount += text.split("\n").filter((line) => line.trim() !== "").length;
  }
  if (tunnelCount === 0) return "Local, remote, and dynamic tunnels";
  return `${tunnelCount} ${tunnelCount === 1 ? "tunnel" : "tunnels"} configured`;
}

// CSS zoom scales pixel lengths; divide viewport bounds to keep a 16px screen inset.
export function connectionDialogBounds(width: number, height: number, scale: number) {
  return { width: (width - 32) / scale, height: (height - 32) / scale, top: 16 / scale };
}

export function revealInvalidConnectionField(form: HTMLFormElement): string | null {
  const emptyRequiredSelect = [...form.querySelectorAll<HTMLSelectElement>("select[required]:not(:disabled)")]
    .find((select) => select.value === "");
  const invalidField = emptyRequiredSelect ?? form.querySelector<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>(":invalid");
  if (!invalidField) return null;

  // Reveal every enclosing disclosure before asking the browser to focus/report.
  let disclosure = invalidField.closest("details");
  while (disclosure) {
    disclosure.open = true;
    disclosure = disclosure.parentElement?.closest("details") ?? null;
  }
  invalidField.focus();
  invalidField.reportValidity();
  return emptyRequiredSelect ? "Choose a saved jump connection." : invalidField.validationMessage;
}
