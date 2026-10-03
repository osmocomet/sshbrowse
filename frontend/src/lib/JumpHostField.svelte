<script lang="ts">
  import { untrack } from "svelte";
  import type { Connection } from "../../bindings/sshbrowse/internal/profile/models";

  let {
    connection = $bindable(),
    connections,
    disabled = false,
    onrawkeydown,
  }: {
    connection: Connection;
    connections: Connection[];
    disabled?: boolean;
    onrawkeydown: (event: KeyboardEvent) => void;
  } = $props();
  let savedMode = $state(untrack(() => !!connection.jumpConnectionId));
  let search = $state("");
  let candidates = $derived(
    connections.filter((item) =>
      item.id !== connection.id &&
      `${item.name} ${item.folder} ${item.user} ${item.host}`.toLowerCase().includes(search.toLowerCase()),
    ),
  );
  let selected = $derived(connections.find((item) => item.id === connection.jumpConnectionId));

  function changeMode() {
    connection.jumpConnectionId = "";
    connection.jumpHost = "";
    search = "";
  }
</script>

<div class="jump-field">
  <label for="jump-mode">Routing method</label>
  <select id="jump-mode" bind:value={savedMode} onchange={changeMode} {disabled}>
    <option value={false}>Enter manually (ProxyJump)</option>
    <option value={true}>Saved connection</option>
  </select>
  {#if savedMode}
    <input aria-label="Search saved jump connections" type="search" onkeydown={(event) => { if (event.key === "Enter") event.preventDefault(); }} bind:value={search} placeholder="Search saved connections" {disabled} />
    <select aria-label="Saved jump connection" bind:value={connection.jumpConnectionId} required {disabled}>
      <option value="">Choose a saved connection…</option>
      {#if connection.jumpConnectionId && !selected}
        <option value={connection.jumpConnectionId} disabled>Saved connection unavailable</option>
      {:else if selected && !candidates.includes(selected)}
        <option value={selected.id}>{selected.folder ? `${selected.folder} / ` : ""}{selected.name} — {selected.host}</option>
      {/if}
      {#each candidates as item (item.id)}
        <option value={item.id}>{item.folder ? `${item.folder} / ` : ""}{item.name} — {item.user ? `${item.user}@` : ""}{item.host}</option>
      {/each}
    </select>
    <p>Uses the saved connection’s SSH settings and your local SSH agent.</p>
  {:else}
    <input id="jump" onkeydown={onrawkeydown} aria-label="Raw ProxyJump" class="technical" bind:value={connection.jumpHost} spellcheck="false" placeholder="[user@]host[:port],…" {disabled} />
    <p>Enter an SSH alias or ProxyJump route. Leave blank to use your SSH routing settings.</p>
  {/if}
</div>

<style>
  .jump-field {
    display: grid;
    gap: 6px;
  }
  label {
    color: var(--text-secondary);
    font-size: var(--ui-font-small);
    font-weight: 500;
  }
  input, select {
    width: 100%;
    box-sizing: border-box;
    font: var(--ui-font-body) var(--font-ui);
    padding: 7px 9px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--input-surface);
    color: var(--text-primary);
  }
  input.technical {
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
  }
  input::placeholder {
    color: var(--text-placeholder);
  }
  input:focus, select:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 2px var(--focus-ring);
  }
  p {
    margin: 4px 0 0;
    color: var(--text-muted);
    font-size: var(--ui-font-small);
    line-height: 1.45;
  }
</style>
