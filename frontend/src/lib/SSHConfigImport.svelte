<script lang="ts">
  import { untrack } from "svelte";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import X from "@lucide/svelte/icons/x";
  import {
    answerSSHConfigImport,
    importSSHConfig,
    type SSHConfigImportResult,
    type SSHConfigIssue,
    type SSHConfigScan,
  } from "./sshconfig";

  let {
    scan,
    firstRun,
    folders,
    initialError = "",
    onimported,
    onclose,
  }: {
    scan: SSHConfigScan;
    firstRun: boolean;
    folders: string[];
    initialError?: string;
    onimported: () => Promise<void>;
    onclose: () => void;
  } = $props();

  let dialog: HTMLDialogElement;
  let primaryButton = $state<HTMLButtonElement>();
  let phase = $state<"prompt" | "choose" | "result">(untrack(() => firstRun) ? "prompt" : "choose");
  let answered = untrack(() => !firstRun);
  let selected = $state<Set<string>>(
    new Set(untrack(() => scan.candidates.filter((candidate) => !candidate.alreadyImported).map((candidate) => candidate.alias))),
  );
  let folder = $state("Imported");
  let error = $state(untrack(() => initialError));
  let working = $state(false);
  let result = $state<SSHConfigImportResult | null>(null);
  let newCandidates = $derived(scan.candidates.filter((candidate) => !candidate.alreadyImported));

  $effect(() => {
    // Each step starts on its default action; Yes only opens the chooser.
    phase;
    dialog.showModal();
    if (!working) {
      primaryButton?.focus();
    }
  });

  function toggle(alias: string) {
    const next = new Set(selected);
    if (next.has(alias)) {
      next.delete(alias);
    } else {
      next.add(alias);
    }
    selected = next;
  }

  async function runImport(aliases: string[]) {
    if (working) {
      return;
    }
    working = true;
    error = "";
    try {
      result = await importSSHConfig(aliases, folder, firstRun);
      phase = "result";
      try {
        await onimported();
      } catch (err) {
        error = `Import completed, but saved connections could not be refreshed: ${String(err)}`;
      }
    } catch (err) {
      error = String(err);
    } finally {
      working = false;
    }
  }

  async function answerPrompt(choose: boolean) {
    if (working) {
      return;
    }
    working = true;
    error = "";
    try {
      if (!answered) {
        await answerSSHConfigImport();
        answered = true;
      }
      if (choose) {
        phase = "choose";
      } else {
        dialog.close();
      }
    } catch (err) {
      error = String(err);
    } finally {
      working = false;
    }
  }

  export function dismiss() {
    void answerPrompt(false);
  }

  function location(issue: SSHConfigIssue): string {
    return `${issue.source}${issue.line > 0 ? `:${issue.line}` : ""}`;
  }
</script>

<dialog bind:this={dialog} onclose={onclose} oncancel={(event) => { event.preventDefault(); dismiss(); }} aria-labelledby="ssh-import-heading">
  <div class="dialog-content">
    <header>
      <div>
        <h2 id="ssh-import-heading">{phase === "prompt" ? "Import your SSH connections?" : "Import from SSH Config"}</h2>
        {#if phase === "choose"}<p>Choose connections to copy into SSHBrowse.</p>{/if}
      </div>
      <button class="close" type="button" aria-label="Close" onclick={dismiss} disabled={working}><X size={16} /></button>
    </header>

    {#if phase === "prompt"}
      <div class="prompt body">
        <p>Found {newCandidates.length} connection{newCandidates.length === 1 ? "" : "s"} in your SSH configuration. Review the list and choose which ones to copy.</p>
        <p>Copied connections are managed here and saved in SSHBrowse's platform-specific configuration directory. Your original SSH configuration stays unchanged.</p>
      </div>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
      <footer>
        <span class="spacer"></span>
        <button type="button" onclick={dismiss} disabled={working}>No</button>
        <button bind:this={primaryButton} type="button" class="primary" onclick={() => answerPrompt(true)} disabled={working}>Yes</button>
      </footer>
    {:else if phase === "choose"}
      <div class="body chooser">
        <p>
          Imported profiles still use your live OpenSSH config when connecting. Command-capable directives such as
          <code>ProxyCommand</code> and <code>LocalCommand</code> still apply.
        </p>
        {#if newCandidates.length > 0}
          <div class="selection-actions">
            <span>{selected.size} of {newCandidates.length} new selected</span>
            <button type="button" disabled={working} onclick={() => (selected = new Set(newCandidates.map((candidate) => candidate.alias)))}>Select all</button>
            <button type="button" disabled={working} onclick={() => (selected = new Set())}>Deselect all</button>
          </div>
        {/if}

        <div class="candidates" aria-label="SSH config connections">
          {#each scan.candidates as candidate (candidate.alias)}
            <label class:imported={candidate.alreadyImported}>
              <input
                type="checkbox"
                checked={selected.has(candidate.alias)}
                disabled={candidate.alreadyImported || working}
                onchange={() => toggle(candidate.alias)}
              />
              <span class="candidate-copy">
                <strong>{candidate.alias}</strong>
              </span>
              {#if candidate.alreadyImported}<span class="status">Imported</span>{/if}
            </label>
          {/each}
          {#if scan.candidates.length === 0}
            <p class="empty">No eligible connections were found.</p>
          {/if}
        </div>

        {#if newCandidates.length > 0}
          <label class="folder">
            <span>Import into folder</span>
            <input bind:value={folder} list="import-folders" disabled={working} spellcheck="false" autocomplete="off" />
            <datalist id="import-folders">
              {#each folders as name}<option value={name}></option>{/each}
            </datalist>
          </label>
        {/if}

        {#if scan.warnings.length > 0}
          <details>
            <summary><ChevronRight class="disclosure-chevron" size={14} />{scan.warnings.length} warning{scan.warnings.length === 1 ? "" : "s"}</summary>
            <div class="issues">
              {#each scan.warnings as issue}
                <p><strong>Warning:</strong> {issue.message}<small>{location(issue)}</small></p>
              {/each}
            </div>
          </details>
        {/if}
      </div>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
      <footer>
        <span class="spacer"></span>
        <button type="button" onclick={dismiss} disabled={working}>Cancel</button>
        <button bind:this={primaryButton} type="button" class="primary" onclick={() => runImport([...selected])} disabled={working || selected.size === 0}>Import selected</button>
      </footer>
    {:else}
      <div class="result body">
        <p><strong>Imported {result?.imported ?? 0} connection{result?.imported === 1 ? "" : "s"}.</strong></p>
        {#if (result?.alreadyImported ?? 0) > 0}<p>{result?.alreadyImported} selected connection{result?.alreadyImported === 1 ? " was" : "s were"} already imported.</p>{/if}
        {#if scan.warnings.length > 0}
          <details>
            <summary><ChevronRight class="disclosure-chevron" size={14} />Warnings</summary>
            <div class="issues">
              {#each scan.warnings as issue}
                <p><strong>Warning:</strong> {issue.message}<small>{location(issue)}</small></p>
              {/each}
            </div>
          </details>
        {/if}
      </div>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
      <footer><span class="spacer"></span><button bind:this={primaryButton} type="button" class="primary" onclick={dismiss} disabled={working}>Done</button></footer>
    {/if}
  </div>
</dialog>

<style>
  dialog {
    width: min(560px, calc(100vw - 32px));
    max-height: min(620px, calc(100vh - 40px));
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
  .dialog-content {
    display: flex;
    max-height: min(620px, calc((100vh - 40px) / var(--interface-scale, 1)));
    flex-direction: column;
  }
  header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    padding: 15px 18px;
  }
  h2,
  header p,
  .body p {
    margin: 0;
  }
  h2 {
    font-size: var(--ui-font-heading);
    letter-spacing: -0.02em;
  }
  header p {
    margin-top: 4px;
    color: var(--text-muted);
  }
  .body {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    padding: 14px 18px;
  }
  .prompt,
  .result {
    display: grid;
    gap: 10px;
    color: var(--text-secondary);
    line-height: 1.5;
  }
  .result strong {
    color: var(--text-primary);
  }
  button,
  .folder input {
    padding: 7px 11px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--input-surface);
    color: var(--text-primary);
    font: var(--ui-font-body) var(--font-ui);
  }
  button:hover:not(:disabled) {
    background: var(--control-hover);
  }
  button:disabled {
    cursor: wait;
    opacity: 0.55;
  }
  .close {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--text-muted);
  }
  .primary {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--accent-foreground);
  }
  button.primary:hover:not(:disabled) {
    background: var(--accent-hover);
  }
  footer {
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
  .selection-actions {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 8px;
    color: var(--text-muted);
    font-size: var(--ui-font-small);
  }
  .selection-actions span {
    flex: 1;
  }
  .selection-actions button {
    padding: 4px 8px;
    font-size: var(--ui-font-tiny);
  }
  .candidates {
    max-height: 280px;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--surface);
  }
  .candidates label {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 45px;
    padding: 7px 10px;
    border-bottom: 1px solid var(--border-subtle);
  }
  .candidates label:last-child {
    border-bottom: 0;
  }
  .candidates label.imported {
    color: var(--text-muted);
  }
  input[type="checkbox"] {
    accent-color: var(--accent);
  }
  .candidate-copy {
    display: grid;
    min-width: 0;
    flex: 1;
    gap: 2px;
  }
  .status {
    color: var(--text-muted);
    font-size: var(--ui-font-tiny);
  }
  .empty {
    padding: 26px;
    color: var(--text-muted);
    text-align: center;
  }
  .folder {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-top: 12px;
    color: var(--text-secondary);
  }
  details {
    margin-top: 14px;
    color: var(--text-secondary);
  }
  summary {
    display: flex;
    align-items: center;
    gap: 4px;
    list-style: none;
    cursor: default;
    color: var(--text-muted);
    font-size: var(--ui-font-small);
  }
  summary::-webkit-details-marker {
    display: none;
  }
  details[open] summary :global(.disclosure-chevron) {
    transform: rotate(90deg);
  }
  .issues {
    display: grid;
    gap: 9px;
    margin-top: 9px;
    padding: 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--input-surface);
  }
  .issues p {
    color: var(--text-secondary);
    font-size: var(--ui-font-small);
    overflow-wrap: anywhere;
  }
  .issues small {
    display: block;
    margin-top: 2px;
    color: var(--text-muted);
    font: var(--ui-font-micro) ui-monospace, "SF Mono", Menlo, monospace;
  }
  .error {
    margin: 0;
    padding: 8px 20px;
    border-top: 1px solid var(--border);
    background: var(--status-error-surface);
    color: var(--status-error);
    font-size: var(--ui-font-small);
  }
</style>
