<script lang="ts">
  import Search from "@lucide/svelte/icons/search";
  import type { Connection } from "../../bindings/sshbrowse/internal/profile/models";
  import { parseDestination } from "./connections.svelte";
  import { errorMessage } from "./errors";
  import { isComposingKey } from "./formKeyboard";
  import { currentPlatform, shortcutLabel } from "./shortcuts";

  let {
    connections,
    onpick,
    onlocal,
    onclose,
  }: {
    connections: Connection[];
    onpick: (connection: Connection) => void;
    onlocal: () => void;
    onclose: () => void;
  } = $props();

  // The list is the typed text as a destination first, then saved matches.
  type Row = { kind: "connect"; text: string } | { kind: "saved"; connection: Connection };

  let dialog: HTMLDialogElement;
  let listbox: HTMLUListElement;
  let query = $state("");
  let selected = $state(0);
  let error = $state("");
  const shortcutPlatform = currentPlatform();
  const newTabShortcut = shortcutLabel("⌘T", "Ctrl+Shift+N", shortcutPlatform);
  const newLocalShortcut = shortcutLabel("⌘⇧T", "Ctrl+Shift+T", shortcutPlatform);

  $effect(() => {
    if (!dialog.open) {
      dialog.showModal();
    }
  });

  export function dismiss() {
    if (dialog.open) {
      dialog.close();
    }
  }

  // Substring matches rank above in-order character matches ("wk1" finds "worker1").
  function rank(connection: Connection, needle: string): number {
    const haystack = `${connection.folder} ${connection.name} ${connection.host}`.toLowerCase();
    if (needle === "" || haystack.includes(needle)) {
      return 0;
    }
    let position = 0;
    for (const character of needle) {
      position = haystack.indexOf(character, position);
      if (position < 0) {
        return -1;
      }
      position++;
    }
    return 1;
  }

  let rows = $derived.by((): Row[] => {
    const text = query.trim();
    const needle = text.toLowerCase();
    const saved: Row[] = connections
      .map((connection, index) => ({ connection, index, rank: rank(connection, needle) }))
      .filter((entry) => entry.rank >= 0)
      .sort((a, b) => a.rank - b.rank || a.index - b.index)
      .map((entry) => ({ kind: "saved", connection: entry.connection }));
    if (text === "") {
      return saved;
    }
    return [{ kind: "connect", text }, ...saved];
  });

  // A saved match is the default choice when there is one; the typed
  // destination is one arrow-up away. With no match, connecting is the default.
  $effect(() => {
    query;
    selected = rows.length > 1 && rows[0].kind === "connect" ? 1 : 0;
    error = "";
  });

  $effect(() => {
    selected;
    rows;
    listbox?.querySelector<HTMLElement>('[aria-selected="true"]')?.scrollIntoView({ block: "nearest" });
  });

  async function pick(row: Row) {
    if (row.kind === "saved") {
      dialog.close();
      onpick(row.connection);
      return;
    }
    try {
      const connection = await parseDestination(row.text);
      dialog.close();
      onpick(connection);
    } catch (err) {
      error = errorMessage(err);
    }
  }

  function onkeydown(event: KeyboardEvent) {
    if (isComposingKey(event)) {
      return;
    }
    if (event.key === "ArrowDown" && rows.length > 0) {
      event.preventDefault();
      selected = Math.min(selected + 1, rows.length - 1);
    } else if (event.key === "ArrowUp" && rows.length > 0) {
      event.preventDefault();
      selected = Math.max(selected - 1, 0);
    } else if (event.key === "Enter") {
      event.preventDefault();
      if (rows[selected]) {
        pick(rows[selected]);
      }
    }
  }
</script>

<dialog bind:this={dialog} onclose={onclose}>
  <div class="heading">
    <div>
      <h2>Open session</h2>
      <p>Choose a saved connection or type a destination.</p>
    </div>
    <kbd>{newTabShortcut}</kbd>
  </div>
  <div class="search">
    <Search class="search-icon" size={16} />
    <!-- svelte-ignore a11y_autofocus -- a modal dialog is the one place autofocus is right -->
    <input bind:value={query} {onkeydown} aria-label="Search connections or enter a host" placeholder="user@host:port" autofocus spellcheck="false" />
  </div>
  <!-- Keyboard handling lives on the input above; the list only mirrors it. -->
  <ul bind:this={listbox} role="listbox">
    {#each rows as row, index (row.kind === "saved" ? row.connection.id : "connect")}
      <li
        role="option"
        aria-selected={index === selected}
        class:selected={index === selected}
        onmousedown={() => pick(row)}
        onmouseenter={() => (selected = index)}
      >
        {#if row.kind === "connect"}
          <span class="row-copy">
            <span class="name">Connect to <b>{row.text}</b></span>
            <span class="detail">New session</span>
          </span>
        {:else}
          <span class="row-copy">
            <span class="name">{row.connection.name}</span>
            <span class="detail">{row.connection.user ? `${row.connection.user}@` : ""}{row.connection.host}{row.connection.port > 0 ? `:${row.connection.port}` : ""}</span>
          </span>
          {#if row.connection.folder}<span class="folder">{row.connection.folder}</span>{/if}
        {/if}
      </li>
    {/each}
    {#if rows.length === 0}
      <li class="none">Nothing saved yet. Type a host to connect.</li>
    {/if}
  </ul>
  {#if error}<p class="error">{error}</p>{/if}
  <div class="footer">
    <span>↑↓ to navigate</span><span>↵ to open</span><span>esc to close</span>
    <button onclick={() => { dialog.close(); onlocal(); }} title={`New local terminal (${newLocalShortcut})`}>Local terminal</button>
  </div>
</dialog>

<style>
  dialog {
    width: min(520px, calc(100vw - 32px));
    margin-top: 10vh;
    padding: 16px;
    border: 1px solid var(--border);
    border-radius: var(--radius-dialog);
    background: var(--surface-overlay);
    color: var(--text-primary);
    font: var(--ui-font-body) var(--font-ui);
    box-shadow: var(--shadow-panel);
  }
  dialog::backdrop {
    background: var(--overlay-backdrop-soft);
  }
  .heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    margin: 0 2px 12px;
  }
  h2 {
    margin: 0;
    color: var(--text-primary);
    font-size: var(--ui-font-body);
    font-weight: 500;
  }
  p {
    margin: 4px 0 0;
    color: var(--text-muted);
    font-size: var(--ui-font-small);
  }
  kbd {
    padding: 3px 7px;
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-muted);
    font-size: var(--ui-font-tiny);
    white-space: nowrap;
  }
  .search {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 11px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--input-surface);
  }
  .search:focus-within {
    border-color: var(--focus-ring);
    box-shadow: 0 0 0 2px var(--focus-ring);
  }
  .search :global(.search-icon) {
    color: var(--text-muted);
  }
  input {
    width: 100%;
    box-sizing: border-box;
    min-width: 0;
    font: var(--ui-font-body) var(--font-ui);
    padding: 11px 0;
    border: 0;
    background: transparent;
    color: var(--text-primary);
    outline: none;
  }
  .search input:focus-visible {
    outline: none;
  }
  ul {
    list-style: none;
    margin: 12px 0 0;
    padding: 0;
    max-height: min(360px, 45vh);
    overflow-y: auto;
  }
  li {
    display: flex;
    justify-content: space-between;
    align-items: center;
    min-height: 42px;
    padding: 6px 10px;
    border: 1px solid transparent;
    border-radius: var(--radius-control);
    cursor: default;
  }
  li.selected {
    background: var(--selection);
    color: var(--text-primary);
  }
  .row-copy {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .detail {
    overflow: hidden;
    color: var(--text-muted);
    font-size: var(--ui-font-small);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .folder {
    flex: none;
    margin-left: 12px;
    color: var(--text-muted);
    font-size: var(--ui-font-tiny);
  }
  .none {
    color: var(--text-muted);
  }
  .error {
    margin: 6px 10px 2px;
    color: var(--status-error);
    font-size: var(--ui-font-small);
  }
  .footer {
    display: flex;
    gap: 14px;
    margin: 12px 2px 0;
    color: var(--text-muted);
    font-size: var(--ui-font-tiny);
    align-items: center;
    flex-wrap: wrap;
  }
  .footer button {
    margin-left: auto;
    padding: 4px 7px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
  }
  .footer button:hover {
    background: var(--control-hover);
    color: var(--text-primary);
  }
</style>
