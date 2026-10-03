<script lang="ts">
  let {
    sessionNames,
    workspace,
    onclose,
  }: {
    sessionNames: string[];
    workspace: boolean;
    onclose: (confirmed: boolean) => void;
  } = $props();

  let dialog: HTMLDialogElement;
  let confirmButton: HTMLButtonElement;

  $effect(() => {
    if (!dialog.open) {
      dialog.showModal();
      confirmButton.focus();
    }
  });

  export function dismiss() {
    if (dialog.open) {
      dialog.close();
    }
  }
</script>

<dialog bind:this={dialog} onclose={() => onclose(dialog.returnValue === "confirm")} aria-labelledby="close-heading">
  <form method="dialog">
    <header>
      <h2 id="close-heading">{workspace ? "Close workspace?" : "Close session?"}</h2>
    </header>
    <div class="body">
      {#if workspace}
        <p>This will close all {sessionNames.length} sessions in this workspace.</p>
        <ul>
          {#each sessionNames as name}
            <li>{name}</li>
          {/each}
        </ul>
      {:else}
        <p>This will close {sessionNames[0] ?? "this session"}.</p>
      {/if}
    </div>
    <footer>
      <button type="button" onclick={() => dialog.close()}>Cancel</button>
      <!-- Enter confirms the initially focused action. -->
      <button bind:this={confirmButton} type="submit" value="confirm" class="primary">
        {workspace ? "Close sessions" : "Close session"}
      </button>
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
  form {
    margin: 0;
  }
  header {
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
  p {
    margin: 0;
    color: var(--text-secondary);
    line-height: 1.45;
  }
  ul {
    max-height: 160px;
    margin: 10px 0 0;
    padding: 0 0 0 18px;
    overflow-y: auto;
    color: var(--text-muted);
  }
  li + li {
    margin-top: 4px;
  }
  footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px 18px;
  }
  button {
    min-height: 30px;
    padding: 0 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--surface);
    color: var(--text-primary);
    font: inherit;
    cursor: default;
  }
  button:hover {
    background: var(--control-hover);
  }
  button.primary {
    border-color: var(--status-error);
    background: var(--status-error);
    color: var(--status-error-foreground);
    font-weight: 500;
  }
  button.primary:hover:not(:disabled) {
    border-color: var(--status-error-hover);
    background: var(--status-error-hover);
  }
  button.primary:focus {
    outline: 2px solid var(--focus-ring);
    outline-offset: 2px;
  }
</style>
