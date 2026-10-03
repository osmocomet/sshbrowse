<script lang="ts">
  import type { UpdateRelease } from "./updates";

  let { release, canDownload, ondownload, onreleasenotes, onlater }: {
    release: UpdateRelease;
    canDownload: boolean;
    ondownload: () => void;
    onreleasenotes: () => void;
    onlater: () => void;
  } = $props();
</script>

<section class="update-notice" aria-label="Update available" aria-live="polite">
  <h2>SSHBrowse {release.version} is available.</h2>
  <p>{canDownload ? "Would you like to download the update? You can restart when you’re ready." : "Install the updated package using your package manager."}</p>
  <a class="release-notes" href={release.releaseURL} onclick={(event) => { event.preventDefault(); onreleasenotes(); }}>Release notes ↗</a>
  <div class="actions">
    <button type="button" onclick={onlater}>Later</button>
    {#if canDownload}
      <button class="primary" type="button" onclick={ondownload}>Download update</button>
    {:else}
      <button class="primary" type="button" onclick={onreleasenotes}>View release</button>
    {/if}
  </div>
</section>

<style>
  .update-notice {
    position: fixed;
    right: 20px;
    bottom: 20px;
    z-index: 20;
    width: min(360px, calc(100vw - 40px));
    box-sizing: border-box;
    padding: 18px;
    border: 1px solid var(--border);
    border-radius: var(--radius-dialog);
    background: var(--surface-overlay);
    color: var(--text-primary);
    font: var(--ui-font-body) var(--font-ui);
    box-shadow: var(--shadow-panel);
  }
  h2 { margin: 0 0 8px; font-size: var(--ui-font-heading); font-weight: 500; }
  p { margin: 0 0 10px; color: var(--text-secondary); line-height: 1.45; }
  button {
    min-height: 30px;
    padding: 0 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--surface);
    color: var(--text-primary);
    font: inherit;
  }
  button:hover { background: var(--control-hover); }
  button:focus-visible, a:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
  .release-notes { padding: 0; border: 0; background: transparent; color: var(--accent); cursor: pointer; text-decoration: none; }
  .release-notes:hover { text-decoration: underline; background: transparent; }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
  .primary { background: var(--accent); border-color: var(--accent); color: var(--accent-foreground); }
  .primary:hover { background: var(--accent-hover); border-color: var(--accent-hover); }
</style>
