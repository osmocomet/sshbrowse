<script lang="ts">
  import { untrack } from "svelte";
  import X from "@lucide/svelte/icons/x";
  import { errorMessage } from "./errors";
  import { folderBaseName, folderParent, joinFolderPath, type FolderAction } from "./folderTree";

  let {
    action,
    oncreate,
    onrename,
    onclose,
  }: {
    action: FolderAction;
    oncreate: (path: string) => Promise<void>;
    onrename: (path: string, newPath: string) => Promise<void>;
    onclose: () => void;
  } = $props();

  let dialog: HTMLDialogElement;
  // The dialog is mounted fresh per request, so only the initial action matters.
  const initial = untrack(() => action);
  const parent = initial.kind === "create" ? initial.parent : folderParent(initial.path);
  let name = $state(initial.kind === "rename" ? folderBaseName(initial.path) : "");
  let error = $state("");
  let submitting = $state(false);

  $effect(() => {
    dialog.showModal();
  });

  export function dismiss() {
    if (!submitting && dialog.open) {
      dialog.close();
    }
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (submitting) {
      return;
    }
    submitting = true;
    error = "";
    try {
      const path = joinFolderPath(parent, name.trim());
      if (initial.kind === "create") {
        await oncreate(path);
      } else {
        await onrename(initial.path, path);
      }
      dialog.close();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      submitting = false;
    }
  }
</script>

<dialog bind:this={dialog} onclose={onclose} oncancel={(event) => { if (submitting) event.preventDefault(); }} aria-labelledby="folder-heading">
  <form onsubmit={submit}>
    <header>
      <h2 id="folder-heading">{initial.kind === "create" ? "New folder" : "Rename folder"}</h2>
      <button class="close" type="button" aria-label="Close" onclick={dismiss} disabled={submitting}><X size={16} /></button>
    </header>
    <div class="body">
      <label class="field" for="folder-name">
        <span>{parent ? `Name, inside ${parent}` : "Name"}</span>
        <!-- svelte-ignore a11y_autofocus -- a modal dialog is the one place autofocus is right -->
        <input id="folder-name" bind:value={name} required autofocus spellcheck="false" autocomplete="off" />
      </label>
      <p class="hint">Use / to create nested folders in one step.</p>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </div>
    <footer>
      <span class="spacer"></span>
      <button type="button" onclick={dismiss} disabled={submitting}>Cancel</button>
      <button type="submit" class="primary" disabled={submitting}>{initial.kind === "create" ? "Create" : "Rename"}</button>
    </footer>
  </form>
</dialog>

<style>
  dialog {
    width: min(420px, calc(100vw - 32px));
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
  header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    padding: 15px 18px;
  }
  h2 {
    margin: 0;
    font-size: var(--ui-font-heading);
    font-weight: 500;
    letter-spacing: -0.02em;
  }
  .body {
    padding: 14px 18px 16px;
  }
  .field {
    display: grid;
    gap: 6px;
  }
  .field input {
    min-height: 36px;
    padding: 7px 9px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--input-surface);
    color: var(--text-primary);
    font: inherit;
  }
  .field input:hover {
    border-color: var(--text-secondary);
  }
  .field input:focus-visible {
    border-color: var(--accent);
  }
  .field > span {
    color: var(--text-secondary);
    font-size: var(--ui-font-small);
    font-weight: 500;
  }
  .hint {
    margin: 8px 0 0;
    color: var(--text-muted);
    font-size: var(--ui-font-small);
  }
  .error {
    margin: 10px 0 0;
    color: var(--status-error);
    font-size: var(--ui-font-small);
  }
  footer {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 16px;
  }
  footer button {
    min-height: 30px;
    padding: 0 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--surface-raised);
    color: var(--text-primary);
    font: inherit;
  }
  footer button:hover:not(:disabled) {
    background: var(--control-hover);
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
  .close:hover {
    background: var(--control-hover);
    color: var(--text-primary);
  }
  .primary {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--accent-foreground);
  }
  footer button.primary:hover:not(:disabled) {
    background: var(--accent-hover);
  }
</style>
