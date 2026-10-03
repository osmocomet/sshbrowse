<script lang="ts">
  let {
    lineCount,
    byteCount,
    recipientCount,
    preview,
    onclose,
  }: {
    lineCount: number;
    byteCount: number;
    recipientCount: number;
    preview: string;
    onclose: (confirmed: boolean) => void;
  } = $props();

  let dialog: HTMLDialogElement;

  $effect(() => {
    if (!dialog.open) {
      dialog.showModal();
    }
  });

  function closeWithoutSending() {
    if (dialog.open) {
      dialog.close();
    }
  }
</script>

<dialog bind:this={dialog} onclose={() => onclose(dialog.returnValue === "confirm")} aria-labelledby="broadcast-confirm-heading">
  <form method="dialog">
    <header>
      <h2 id="broadcast-confirm-heading">Confirm command send</h2>
    </header>
    <div class="body">
      <p>Send {lineCount} command lines to {recipientCount} session{recipientCount === 1 ? "" : "s"}?</p>
      <dl>
        <div><dt>Bytes</dt><dd>{byteCount.toLocaleString()}</dd></div>
        <div><dt>Lines</dt><dd>{lineCount}</dd></div>
      </dl>
      <pre aria-label="Command preview">{preview}</pre>
      <p class="warning">Only send when every selected session is at a command prompt.</p>
    </div>
    <footer>
      <button type="button" onclick={closeWithoutSending}>Cancel</button>
      <!-- svelte-ignore a11y_autofocus -- Enter should confirm this deliberate staged send. -->
      <button type="submit" value="confirm" class="primary" autofocus>Send</button>
    </footer>
  </form>
</dialog>

<style>
  dialog {
    width: min(500px, calc(100vw - 32px));
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
  dl {
    display: flex;
    gap: 18px;
    margin: 12px 0;
  }
  dl div {
    display: flex;
    gap: 6px;
  }
  dt {
    color: var(--text-muted);
  }
  dd {
    margin: 0;
    color: var(--text-primary);
    font-weight: 500;
  }
  pre {
    max-height: 180px;
    margin: 0 0 12px;
    padding: 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--surface);
    color: var(--text-primary);
    font: var(--ui-font-small)/1.45 ui-monospace, "SF Mono", Menlo, monospace;
    white-space: pre-wrap;
    overflow: auto;
  }
  .warning {
    color: var(--status-warning);
    font-size: var(--ui-font-small);
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
    border-color: var(--accent);
    background: var(--accent);
    color: var(--accent-foreground);
    font-weight: 500;
  }
  button.primary:hover:not(:disabled) {
    background: var(--accent-hover);
  }
</style>
