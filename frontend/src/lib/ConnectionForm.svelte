<script lang="ts">
  import { connectionFormSections } from "./connectionForm";
  import JumpHostField from "./JumpHostField.svelte";
  import { untrack } from "svelte";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import X from "@lucide/svelte/icons/x";
  import * as ConnectionsService from "../../bindings/sshbrowse/internal/app/connections";
  import type { Connection } from "../../bindings/sshbrowse/internal/profile/models";
  import type { ConnectionFormMode } from "./connections.svelte";
  import { shouldSaveOnEnter, type FormEnterTarget } from "./formKeyboard";

  let {
    connection,
    mode,
    folders,
    connections,
    onsave,
    ondelete,
    onclose,
  }: {
    connection: Connection;
    mode: ConnectionFormMode;
    folders: string[];
    connections: Connection[];
    onsave: (connection: Connection, connectAfterSave: boolean) => Promise<void>;
    ondelete: (id: string) => Promise<void>;
    onclose: () => void;
  } = $props();

  let dialog: HTMLDialogElement;
  let form: HTMLFormElement;
  let hostInput: HTMLInputElement;
  let portInput: HTMLInputElement;
  let saveButton: HTMLButtonElement;
  // Edits happen on a copy so Cancel leaves the original untouched. The form is
  // mounted fresh per connection, so only the initial value matters; untrack
  // tells Svelte that is intended.
  const initial = untrack(() => structuredClone($state.snapshot(connection)));
  let draft = $state<Connection>(initial);
  // Port is edited as text so an unset port shows as empty, not 0.
  let portText = $state(initial.port > 0 ? String(initial.port) : "");
  let localForwards = $state((initial.localForwards ?? []).join("\n"));
  let remoteForwards = $state((initial.remoteForwards ?? []).join("\n"));
  let dynamicForwards = $state((initial.dynamicForwards ?? []).join("\n"));
  let error = $state("");
  let submitting = $state(false);
  let choosingIdentity = $state(false);

  // A duplicate creates a new saved profile too; keep its existing connect action.
  let canConnectAfterSave = $derived(mode === "new" || mode === "duplicate");
  const headings: Record<ConnectionFormMode, string> = {
    new: "New connection",
    edit: "Edit connection",
    bookmark: "Save connection",
    duplicate: "Duplicate connection",
  };
  let heading = $derived(headings[mode]);

  const initialSections = connectionFormSections(initial);
  let authOpen = $state(initialSections.authentication);
  let routingOpen = $state(initialSections.routing);
  let forwardingOpen = $state(initialSections.forwarding);
  let tunnelsOpen = $state(initialSections.tunnels);

  $effect(() => {
    dialog.showModal();
  });

  export function dismiss() {
    if (!submitting && !choosingIdentity && dialog.open) {
      dialog.close();
    }
  }

  function lines(text: string): string[] {
    return text.split("\n");
  }

  function validate(): boolean {
    const host = draft.host.trim();
    const port = portText.trim();
    if (host === "") {
      error = "Host is required.";
      hostInput.focus();
      return false;
    }
    if (port !== "" && (!/^[0-9]+$/.test(port) || Number(port) < 1 || Number(port) > 65535)) {
      error = "Port must be a whole number from 1 to 65535.";
      portInput.focus();
      return false;
    }
    return form.reportValidity();
  }

  function saveOnEnter(event: KeyboardEvent) {
    const target = event.target;
    const enterTarget: FormEnterTarget =
      target instanceof HTMLInputElement
        ? { kind: "input", type: target.type }
        : target instanceof HTMLTextAreaElement
          ? { kind: "textarea" }
          : target instanceof HTMLButtonElement
            ? { kind: "button" }
            : null;
    if (!shouldSaveOnEnter(event, enterTarget)) {
      return;
    }
    event.preventDefault();
    form.requestSubmit(saveButton);
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (submitting || !validate()) {
      return;
    }

    const port = portText.trim();
    draft.host = draft.host.trim();
    draft.port = port === "" ? 0 : Number(port);
    draft.localForwards = lines(localForwards);
    draft.remoteForwards = lines(remoteForwards);
    draft.dynamicForwards = lines(dynamicForwards);
    const submitter = event.submitter;
    const connectAfterSave =
      canConnectAfterSave && submitter instanceof HTMLButtonElement && submitter.dataset.connect === "true";

    submitting = true;
    error = "";
    try {
      await onsave($state.snapshot(draft), connectAfterSave);
      dialog.close(); // fires the dialog's close event, which calls onclose
    } catch (err) {
      error = String(err);
    } finally {
      submitting = false;
    }
  }

  async function remove() {
    if (submitting) {
      return;
    }
    submitting = true;
    error = "";
    try {
      await ondelete(draft.id);
      dialog.close();
    } catch (err) {
      error = String(err);
    } finally {
      submitting = false;
    }
  }

  async function chooseIdentityFile() {
    if (choosingIdentity || submitting) {
      return;
    }
    choosingIdentity = true;
    error = "";
    try {
      const path = await ConnectionsService.ChooseIdentityFile();
      if (path) {
        draft.identityFile = path;
      }
    } catch (err) {
      error = String(err);
    } finally {
      choosingIdentity = false;
    }
  }
</script>

<dialog
  bind:this={dialog}
  onclose={onclose}
  oncancel={(event) => { if (submitting || choosingIdentity) event.preventDefault(); }}
  aria-labelledby="connection-heading"
>
  <form bind:this={form} onsubmit={submit}>
    <header>
      <h2 id="connection-heading">{heading}</h2>
      <button class="close" type="button" aria-label="Close" onclick={dismiss} disabled={submitting || choosingIdentity}><X size={16} /></button>
    </header>

    <div class="body">
      <section class="section" aria-labelledby="connection-section-heading">
        <div class="section-heading"><h3 id="connection-section-heading">Connection</h3></div>
        <label class="field" for="host">
          <span>Host <em>Required</em></span>
          <!-- svelte-ignore a11y_autofocus -- a modal dialog is the one place autofocus is right -->
          <input
            bind:this={hostInput}
            class="technical"
            id="host"
            bind:value={draft.host}
            required
            autofocus={mode !== "bookmark"}
            placeholder="Hostname, IP address, or SSH alias"
            spellcheck="false"
            autocomplete="off"
            onkeydown={saveOnEnter}
          />
        </label>

        <div class="row">
          <label class="field" for="user">
            <span>Username</span>
            <input id="user" class="technical" bind:value={draft.user} placeholder="Optional" spellcheck="false" autocomplete="username" aria-describedby="port-hint" onkeydown={saveOnEnter} />
          </label>
          <label class="field port-field" for="port">
            <span>Port</span>
            <input
              bind:this={portInput}
              class="technical"
              id="port"
              bind:value={portText}
              inputmode="numeric"
              placeholder="Default"
              aria-describedby="port-hint"
              onkeydown={saveOnEnter}
            />
          </label>
        </div>
        <p class="hint" id="port-hint">When blank, SSH uses your configured username and port, or your local username and port 22.</p>

        <div class="row organization-row">
          <label class="field" for="name">
            <span>Name</span>
            <!-- svelte-ignore a11y_autofocus -->
            <input id="name" bind:value={draft.name} autofocus={mode === "bookmark"} placeholder={draft.host || "Defaults to host"} onkeydown={saveOnEnter} />
          </label>
          <label class="field" for="folder">
            <span>Folder</span>
            <input id="folder" bind:value={draft.folder} list="folders" placeholder="No folder" aria-describedby="folder-hint" onkeydown={saveOnEnter} />
          </label>
        </div>
        <p class="hint" id="folder-hint">Name defaults to Host. Choose or create a folder; use / for nested folders.</p>
        <datalist id="folders">
          {#each folders as folder}<option value={folder}></option>{/each}
        </datalist>
      </section>

      <details class="section disclosure" bind:open={authOpen}>
        <summary>
          <span>
            <strong>Authentication</strong>
            <small>{draft.identityFile ? "Custom identity file" : "SSH keys and password prompts"}</small>
          </span>
          <ChevronDown class="chevron" size={14} />
        </summary>
        <div class="details-body">
          <label class="field" for="identity">
            <span>SSH key file</span>
            <div class="picker-field">
              <input id="identity" aria-describedby="identity-hint" class="technical" bind:value={draft.identityFile} spellcheck="false" placeholder="~/.ssh/id_ed25519" onkeydown={saveOnEnter} />
              <button type="button" onclick={chooseIdentityFile} disabled={choosingIdentity || submitting}>
                {choosingIdentity ? "Choosing…" : "Choose…"}
              </button>
            </div>
          </label>
          <p class="hint" id="identity-hint">Leave blank to use your usual SSH keys and authentication. SSH asks for passwords and key passphrases in the terminal.</p>
        </div>
      </details>

      <details class="section disclosure" bind:open={routingOpen}>
        <summary>
          <span>
            <strong>Jump host</strong>
            <small>{draft.jumpConnectionId ? "Via a saved connection" : draft.jumpHost || "Connect through another host"}</small>
          </span>
          <ChevronDown class="chevron" size={14} />
        </summary>
        <div class="details-body">
          <JumpHostField bind:connection={draft} {connections} disabled={submitting} onrawkeydown={saveOnEnter} />
        </div>
      </details>

      <details class="section disclosure" bind:open={forwardingOpen}>
        <summary>
          <span>
            <strong>Agent and X11 forwarding</strong>
            <small>Use your local SSH agent or X11 display remotely</small>
          </span>
          <ChevronDown class="chevron" size={14} />
        </summary>
        <div class="details-body">
          <div class="checks">
            <label><input type="checkbox" bind:checked={draft.agentForwarding} disabled={submitting} aria-describedby="agent-hint" /> Forward SSH agent</label>
            <label><input type="checkbox" bind:checked={draft.x11Forwarding} disabled={submitting} /> Forward X11</label>
          </div>
          <p class="hint" id="agent-hint">Agent forwarding lets the remote host use your local SSH agent.</p>
        </div>
      </details>

      <details class="section disclosure" bind:open={tunnelsOpen}>
        <summary>
          <span>
            <strong>Port forwarding</strong>
            <small>Local, remote, and dynamic tunnels</small>
          </span>
          <ChevronDown class="chevron" size={14} />
        </summary>
        <div class="details-body">
          <label class="field" for="local">
            <span>Local forwarding</span>
            <textarea id="local" class="technical" bind:value={localForwards} rows="2" spellcheck="false" placeholder="8080:localhost:80"></textarea>
          </label>
          <label class="field" for="remote">
            <span>Remote forwarding</span>
            <textarea id="remote" class="technical" bind:value={remoteForwards} rows="2" spellcheck="false" placeholder="9090:localhost:90"></textarea>
          </label>
          <label class="field" for="dynamic">
            <span>Dynamic forwarding</span>
            <textarea id="dynamic" class="technical" bind:value={dynamicForwards} rows="2" spellcheck="false" placeholder="1080"></textarea>
          </label>
          <p class="hint">One tunnel per line, using the format after <code>-L</code>, <code>-R</code>, or <code>-D</code>.</p>
          <p class="hint">Non-loopback bind addresses can expose forwarded listeners to other machines.</p>
        </div>
      </details>

    </div>

    {#if error}<p class="error" role="alert">{error}</p>{/if}

    <footer class="actions">
      {#if mode === "edit"}<button type="button" class="danger" onclick={remove} disabled={submitting}>Delete</button>{/if}
      <span class="spacer"></span>
      <button type="button" onclick={dismiss} disabled={submitting || choosingIdentity}>Cancel</button>
      {#if canConnectAfterSave}
        <button type="submit" class="secondary" data-connect="true" disabled={submitting}>Save &amp; connect</button>
      {/if}
      <button type="submit" bind:this={saveButton} class="primary" disabled={submitting}>Save</button>
    </footer>
  </form>
</dialog>

<style>
  dialog {
    width: min(580px, calc(100vw - 32px));
    height: auto;
    max-height: calc(100vh - 32px);
    top: 16px;
    bottom: auto;
    margin: 0 auto;
    box-sizing: border-box;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: var(--radius-dialog);
    background: var(--surface-overlay);
    color: var(--text-primary);
    font: var(--ui-font-body) var(--font-ui);
    box-shadow: var(--shadow-panel);
    overflow: hidden;
  }
  dialog::backdrop {
    background: var(--overlay-backdrop);
  }
  form {
    display: flex;
    flex-direction: column;
    max-height: calc(100vh - 34px);
    min-height: 0;
  }
  header {
    flex: none;
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    padding: 15px 18px;
    background: var(--surface-raised);
  }
  h2 {
    margin: 0;
    color: var(--text-primary);
    font-size: var(--ui-font-heading);
    font-weight: 500;
    letter-spacing: -0.02em;
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 8px 18px 12px;
    scrollbar-gutter: stable;
    scrollbar-color: var(--border) transparent;
  }
  .section {
    margin: 0 0 8px;
    padding: 11px 4px 13px;
    border: 0;
    border-bottom: 1px solid var(--border-subtle);
    border-radius: 0;
    background: transparent;
    box-shadow: none;
  }
  .section-heading {
    margin-bottom: 10px;
  }
  h3 {
    margin: 0;
    color: var(--text-primary);
    font-size: var(--ui-font-body);
    font-weight: 500;
  }
  .hint {
    margin: 4px 0 0;
    color: var(--text-muted);
    font-size: var(--ui-font-small);
    line-height: 1.45;
  }
  .field {
    display: grid;
    gap: 6px;
    min-width: 0;
  }
  .field > span {
    color: var(--text-secondary);
    font-size: var(--ui-font-small);
    font-weight: 500;
  }
  em {
    margin-left: 4px;
    color: var(--text-muted);
    font-size: var(--ui-font-tiny);
    font-style: normal;
    font-weight: 400;
  }
  .row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 148px;
    gap: 12px;
    margin-top: 12px;
  }
  .organization-row {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    margin-top: 18px;
  }
  input,
  textarea {
    width: 100%;
    box-sizing: border-box;
    font: var(--ui-font-body) var(--font-ui);
    padding: 7px 9px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--input-surface);
    color: var(--text-primary);
    resize: vertical;
    transition: border-color 120ms ease, box-shadow 120ms ease, background 120ms ease;
  }
  input::placeholder,
  textarea::placeholder {
    color: var(--text-placeholder);
  }
  input.technical,
  textarea.technical {
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
  }
  input:hover,
  textarea:hover {
    border-color: var(--text-secondary);
  }
  input:focus,
  textarea:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 2px var(--focus-ring);
    background: var(--surface-raised);
  }
  .hint {
    margin-top: 10px;
  }
  code {
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
    font-size: var(--ui-font-tiny);
    color: var(--text-secondary);
  }
  .disclosure {
    padding: 0;
    overflow: hidden;
  }
  summary {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 12px 4px;
    cursor: default;
    list-style: none;
    user-select: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  summary:hover {
    background: var(--control-hover);
  }
  summary span:first-child {
    display: grid;
    gap: 3px;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  summary strong {
    color: var(--text-primary);
    font-size: var(--ui-font-body);
    font-weight: 500;
  }
  summary small {
    color: var(--text-muted);
    font-size: var(--ui-font-small);
  }
  details :global(.chevron) {
    color: var(--text-secondary);
    transition: transform 120ms ease;
  }
  details[open] :global(.chevron) {
    transform: rotate(180deg);
  }
  .details-body {
    display: grid;
    gap: 10px;
    padding: 0 4px 12px;
  }
  .picker-field {
    display: flex;
    gap: 8px;
  }
  .picker-field input {
    min-width: 0;
  }
  button {
    flex: none;
    padding: 7px 11px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--surface-raised);
    color: var(--text-primary);
    font: var(--ui-font-body) var(--font-ui);
    cursor: default;
    transition: background 120ms ease, border-color 120ms ease;
  }
  button:hover:not(:disabled) {
    background: var(--control-hover);
  }
  summary:focus-visible,
  button:focus-visible {
    outline: none;
    box-shadow: 0 0 0 2px var(--focus-ring);
  }
  button:disabled {
    cursor: wait;
    opacity: 0.55;
  }
  .checks {
    display: flex;
    flex-wrap: wrap;
    gap: 10px 18px;
  }
  .checks label {
    display: flex;
    align-items: center;
    gap: 7px;
    color: var(--text-secondary);
    font-size: var(--ui-font-small);
  }
  .checks input {
    width: 14px;
    height: 14px;
    margin: 0;
    accent-color: var(--accent);
  }
  .error {
    flex: none;
    margin: 0;
    padding: 8px 20px;
    border-top: 1px solid var(--border);
    background: var(--status-error-surface);
    color: var(--status-error);
    font-size: var(--ui-font-small);
  }
  footer.actions {
    flex: none;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 16px;
    border-top: 1px solid var(--border);
    background: var(--surface-raised);
  }
  .spacer {
    flex: 1;
  }
  .close {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-control);
    background: transparent;
    color: var(--text-muted);
  }
  .close:hover:not(:disabled) {
    border: 0;
    background: var(--control-hover);
    color: var(--text-primary);
  }
  .primary {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--accent-foreground);
  }
  .secondary {
    background: var(--surface-raised);
    color: var(--text-primary);
  }
  .secondary:hover:not(:disabled) {
    background: var(--control-hover);
  }
  .primary:hover:not(:disabled) {
    border-color: var(--accent);
    background: var(--accent-hover);
    color: var(--accent-foreground);
  }
  .danger {
    border-color: transparent;
    background: transparent;
    color: var(--status-error);
  }
  .danger:hover:not(:disabled) {
    border-color: var(--status-error);
    background: var(--status-error-surface);
  }
  @media (max-width: 520px) {
    .row {
      grid-template-columns: 1fr;
    }
    footer.actions {
      flex-wrap: wrap;
    }
    footer .spacer {
      display: none;
    }
  }
</style>
