<script lang="ts">
  import { onDestroy, onMount, tick } from "svelte";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import X from "@lucide/svelte/icons/x";
  import BroadcastCommandConfirmDialog from "./BroadcastCommandConfirmDialog.svelte";
  import {
    broadcastCommandLineCount,
    broadcastCommandPreview,
    broadcastCommandError,
    broadcastRecipients,
    maximumBroadcastCommandBytes,
    snapshotBroadcastRecipients,
    type BroadcastDelivery,
    type BroadcastScope,
    type BroadcastSnapshot,
  } from "./routing";
  import { currentPlatform, type ShortcutPlatform } from "./shortcuts";
  import { utf8ByteLength } from "./terminalInput";
  import type { LiveBroadcastState } from "./liveBroadcast";
  import type { Session, Tab } from "./tabs";

  let {
    sessions,
    tabs,
    displayLabels,
    activeId,
    enabled,
    liveBroadcast,
    broadcastError,
    sendCommand,
    startLiveBroadcast,
    stopLiveBroadcast,
    onclose,
    onsendingchange,
    onconfirmationchange,
  }: {
    sessions: Session[];
    tabs: Tab[];
    displayLabels: ReadonlyMap<number, string>;
    activeId: number | null;
    enabled: boolean;
    liveBroadcast: LiveBroadcastState | null;
    broadcastError: string;
    sendCommand: (recipients: BroadcastSnapshot[], command: string) => Promise<BroadcastDelivery[]>;
    startLiveBroadcast: (snapshot: BroadcastSnapshot[]) => string | null;
    stopLiveBroadcast: () => void;
    onclose: () => void;
    onsendingchange: (sending: boolean) => void;
    onconfirmationchange: (confirming: boolean) => void;
  } = $props();

  type BroadcastMode = "command" | "live";

  let mode = $state<BroadcastMode>("command");
  let scope = $state<BroadcastScope>("current");
  let command = $state("");
  let excludedSessionIds = $state<number[]>([]);
  let sending = $state(false);
  let validationError = $state<string | null>(null);
  let deliveries = $state<BroadcastDelivery[] | null>(null);
  let commandInput = $state<HTMLTextAreaElement>();
  let recipientPicker = $state<HTMLDetailsElement>();
  let commandConfirmation = $state<{
    recipients: BroadcastSnapshot[];
    command: string;
    lineCount: number;
    byteCount: number;
    preview: string;
  } | null>(null);
  const shortcutPlatform: ShortcutPlatform = currentPlatform();
  const recipients = $derived(broadcastRecipients(sessions, tabs, activeId, scope, displayLabels));
  const selectedRecipients = $derived(
    recipients.filter((recipient) => recipient.available && !excludedSessionIds.includes(recipient.logicalSessionId)),
  );
  const selectedCount = $derived(selectedRecipients.length);
  const recipientSummary = $derived(
    `${selectedRecipients.slice(0, 2).map((recipient) => recipient.label).join(", ")}${selectedCount > 2 ? ` +${selectedCount - 2}` : ""}`,
  );

  onMount(() => {
    if (enabled && mode === "command") commandInput?.focus();
  });

  onDestroy(() => {
    if (commandConfirmation !== null) {
      onconfirmationchange(false);
    }
  });

  function setMode(nextMode: BroadcastMode) {
    if (liveBroadcast !== null || mode === nextMode) {
      return;
    }
    mode = nextMode;
    if (recipientPicker) recipientPicker.open = false;
    validationError = null;
    deliveries = null;
    void tick().then(() => {
      if (enabled && mode === "command") commandInput?.focus();
    });
  }

  function setScope(nextScope: BroadcastScope) {
    scope = nextScope;
    if (recipientPicker) recipientPicker.open = false;
    excludedSessionIds = [];
    validationError = null;
    deliveries = null;
  }

  function setIncluded(logicalSessionId: number, included: boolean) {
    if (included) {
      excludedSessionIds = excludedSessionIds.filter((id) => id !== logicalSessionId);
    } else if (!excludedSessionIds.includes(logicalSessionId)) {
      excludedSessionIds = [...excludedSessionIds, logicalSessionId];
    }
    deliveries = null;
  }

  async function sendSnapshot(snapshot: BroadcastSnapshot[], commandToSend: string) {
    if (sending) {
      return;
    }
    sending = true;
    onsendingchange(true);
    deliveries = null;
    try {
      deliveries = await sendCommand(snapshot, commandToSend);
      if (deliveries.every((delivery) => delivery.error === null)) {
        command = "";
      }
    } catch (error) {
      validationError = String(error);
    } finally {
      sending = false;
      onsendingchange(false);
      await tick();
      if (enabled && mode === "command") commandInput?.focus();
    }
  }

  async function submit() {
    if (sending || commandConfirmation !== null || !enabled) {
      return;
    }
    validationError = broadcastCommandError(command);
    if (validationError !== null) {
      return;
    }
    const snapshot = snapshotBroadcastRecipients(recipients, new Set(excludedSessionIds));
    if (recipientPicker) recipientPicker.open = false;
    if (snapshot.length === 0) {
      validationError = "Select at least one available session.";
      return;
    }

    if (broadcastCommandLineCount(command) > 1) {
      commandConfirmation = {
        recipients: snapshot,
        command,
        lineCount: broadcastCommandLineCount(command),
        byteCount: utf8ByteLength(command),
        preview: broadcastCommandPreview(command),
      };
      onconfirmationchange(true);
      return;
    }
    await sendSnapshot(snapshot, command);
  }

  function startLive() {
    if (!enabled || liveBroadcast !== null) {
      return;
    }
    const snapshot = snapshotBroadcastRecipients(recipients, new Set(excludedSessionIds));
    if (recipientPicker) recipientPicker.open = false;
    validationError = startLiveBroadcast(snapshot);
  }

  function finishConfirmation(confirmed: boolean) {
    const request = commandConfirmation;
    commandConfirmation = null;
    onconfirmationchange(false);
    if (confirmed && request !== null) {
      void sendSnapshot(request.recipients, request.command);
    } else {
      void tick().then(() => {
        if (enabled && mode === "command") commandInput?.focus();
      });
    }
  }

  function isSendShortcut(event: KeyboardEvent): boolean {
    if (event.key !== "Enter" || event.shiftKey || event.altKey) {
      return false;
    }
    return shortcutPlatform === "mac"
      ? event.metaKey && !event.ctrlKey
      : event.ctrlKey && !event.metaKey;
  }

  function handleCommandKeydown(event: KeyboardEvent) {
    if (isSendShortcut(event)) {
      event.preventDefault();
      void submit();
    }
  }

  function closeOnEscape(event: KeyboardEvent) {
    if (event.key === "Escape" && recipientPicker?.open) {
      event.preventDefault();
      event.stopPropagation();
      recipientPicker.open = false;
      return;
    }
    if (
      event.key === "Escape" &&
      mode === "command" &&
      liveBroadcast === null &&
      enabled &&
      commandConfirmation === null &&
      !sending &&
      event.target instanceof Element &&
      event.target.closest(".broadcast-bar")
    ) {
      event.preventDefault();
      event.stopPropagation();
      onclose();
    }
  }

  function closePickerOutside(event: PointerEvent) {
    if (recipientPicker?.open && event.target instanceof Node && !recipientPicker.contains(event.target)) {
      recipientPicker.open = false;
    }
  }

  function validatePaste(event: ClipboardEvent) {
    const pasted = event.clipboardData?.getData("text");
    const input = commandInput;
    if (!pasted || !input) return;
    const start = input.selectionStart ?? command.length;
    const end = input.selectionEnd ?? start;
    const candidate = command.slice(0, start) + pasted + command.slice(end);
    const error = broadcastCommandError(candidate);
    if (error !== null) {
      event.preventDefault();
      validationError = error;
      deliveries = null;
    }
  }
</script>

<svelte:window onkeydown={closeOnEscape} onpointerdown={closePickerOutside} />

<section id="broadcast-command-bar" class="broadcast-bar" aria-label="Broadcast">
  {#if liveBroadcast}
    <div class="live-strip" role="status">
      <strong>LIVE INPUT · {liveBroadcast.snapshot.length} terminals</strong>
      <span>Every keystroke is mirrored</span>
      <button class="stop-live" type="button" onpointerdown={(event) => event.preventDefault()} onclick={stopLiveBroadcast}>Stop</button>
    </div>
    {#if broadcastError}
      <div class="broadcast-status" role="alert"><span class="error">{broadcastError}</span></div>
    {/if}
  {:else}
    <div class="broadcast-heading">
      <div class="mode-switch" role="group" aria-label="Broadcast mode">
        <button type="button" class:active={mode === "command"} aria-pressed={mode === "command"} onclick={() => setMode("command")}>Command</button>
        <button type="button" class:active={mode === "live"} aria-pressed={mode === "live"} onclick={() => setMode("live")}>Live input</button>
      </div>
      <button class="close" type="button" aria-label="Close broadcast bar" title="Close" disabled={sending || commandConfirmation !== null} onclick={onclose}><X size={16} /></button>
    </div>

    {#if mode === "command"}
      <form class="broadcast-form" onsubmit={(event) => { event.preventDefault(); void submit(); }}>
        <textarea
          class="command"
          bind:this={commandInput}
          bind:value={command}
          maxlength={maximumBroadcastCommandBytes}
          autocomplete="off"
          spellcheck="false"
          aria-label="Command"
          rows="2"
          placeholder={shortcutPlatform === "mac" ? "Command block — ⌘↵ to send" : "Command block — Ctrl+↵ to send"}
          disabled={sending || commandConfirmation !== null}
          onpaste={validatePaste}
          onkeydown={handleCommandKeydown}
          oninput={() => { validationError = null; deliveries = null; }}
        ></textarea>

        <div class="destination-row">
          <div class="scope" role="group" aria-label="Recipient scope">
            <button type="button" class:active={scope === "current"} aria-pressed={scope === "current"} disabled={sending} onclick={() => setScope("current")}>Current tab</button>
            <button type="button" class:active={scope === "all"} aria-pressed={scope === "all"} disabled={sending} onclick={() => setScope("all")}>All tabs</button>
          </div>
          {@render RecipientPicker(recipients, excludedSessionIds, sending, setIncluded)}
          <button class="send" type="submit" disabled={sending || selectedCount === 0 || command.trim() === ""}>{sending ? "Sending…" : selectedCount === 0 ? "Send" : `Send to ${selectedCount}`}</button>
        </div>
      </form>

      <div class="broadcast-status" aria-live="polite">
        <span class="command-caution">Send only at a command prompt.</span>
        {#if validationError}
          <span class="error">{validationError}</span>
        {:else if deliveries}
          {@const sentCount = deliveries.filter((delivery) => delivery.error === null).length}
          {@const failures = deliveries.filter((delivery) => delivery.error !== null)}
          <span>{sentCount > 0 ? `Sent to ${sentCount} session${sentCount === 1 ? "" : "s"}.` : "No deliveries confirmed."}</span>
          {#each failures as failure (failure.logicalSessionId)}
            <span class="error">{failure.label}: {failure.error}</span>
          {/each}
        {/if}
      </div>
    {:else}
      <div class="live-setup">
        <div class="live-warning" role="alert">
          <strong>Every keystroke is mirrored.</strong>
          <span>Control keys and approved paste go to each selected terminal, including the focused one.</span>
        </div>
        <div class="destination-row">
          <div class="scope" role="group" aria-label="Recipient scope">
            <button type="button" class:active={scope === "current"} aria-pressed={scope === "current"} onclick={() => setScope("current")}>Current tab</button>
            <button type="button" class:active={scope === "all"} aria-pressed={scope === "all"} onclick={() => setScope("all")}>All tabs</button>
          </div>
          {@render RecipientPicker(recipients, excludedSessionIds, false, setIncluded)}
          <button class="start-live" type="button" disabled={!enabled || selectedCount < 2} onclick={startLive}>Start live input</button>
        </div>
      </div>
      <div class="broadcast-status" aria-live="polite">
        {#if selectedCount < 2}<span class="reminder">Select at least two live sessions, including the focused terminal.</span>{/if}
        {#if validationError}<span class="error">{validationError}</span>{/if}
        {#if broadcastError}<span class="error">{broadcastError}</span>{/if}
      </div>
    {/if}
  {/if}
</section>

{#if commandConfirmation}
  <BroadcastCommandConfirmDialog
    lineCount={commandConfirmation.lineCount}
    byteCount={commandConfirmation.byteCount}
    recipientCount={commandConfirmation.recipients.length}
    preview={commandConfirmation.preview}
    onclose={finishConfirmation}
  />
{/if}

{#snippet RecipientPicker(
  recipients: ReturnType<typeof broadcastRecipients>,
  excludedSessionIds: number[],
  sending: boolean,
  setIncluded: (logicalSessionId: number, included: boolean) => void,
)}
  <details class="recipient-picker" bind:this={recipientPicker}>
    <summary title={selectedRecipients.map((recipient) => recipient.label).join(", ")}>
      {#if selectedCount === 0}
        <span class="recipient-count" aria-live="polite">Choose recipients</span>
      {:else if selectedCount === 1}
        <span class="recipient-summary" aria-live="polite">{recipientSummary}</span>
      {:else}
        <span class="recipient-count" aria-live="polite">{selectedCount} recipient{selectedCount === 1 ? "" : "s"}</span>
        <span class="recipient-summary">{recipientSummary}</span>
      {/if}
      <ChevronDown class="picker-chevron" size={14} />
    </summary>
    <div class="recipient-menu">
      {@render RecipientList(recipients, excludedSessionIds, sending, setIncluded)}
    </div>
  </details>
{/snippet}

{#snippet RecipientList(
  recipients: ReturnType<typeof broadcastRecipients>,
  excludedSessionIds: number[],
  sending: boolean,
  setIncluded: (logicalSessionId: number, included: boolean) => void,
)}
  <div class="recipient-list" aria-label="Broadcast recipients">
    {#each recipients as recipient (recipient.logicalSessionId)}
      <label class:unavailable={!recipient.available} title={recipient.available ? recipient.label : `${recipient.label}: ${recipient.status}`}>
        <input
          type="checkbox"
          checked={recipient.available && !excludedSessionIds.includes(recipient.logicalSessionId)}
          disabled={!recipient.available || sending}
          onchange={(event) => setIncluded(recipient.logicalSessionId, event.currentTarget.checked)}
        />
        <span class="recipient-name">{recipient.label}</span>
        {#if !recipient.available}<small class="recipient-status">{recipient.status}</small>{/if}
      </label>
    {:else}
      <span class="no-recipients">No sessions in scope</span>
    {/each}
  </div>
{/snippet}

<style>
  .broadcast-bar {
    container-type: inline-size;
    position: relative;
    z-index: 3;
    flex: none;
    display: grid;
    gap: 6px;
    padding: 6px 12px;
    border-bottom: 1px solid var(--border-subtle);
    background: var(--surface);
    color: var(--text-secondary);
    font: var(--ui-font-small) var(--font-ui);
  }
  .broadcast-heading,
  .destination-row,
  .live-strip,
  .mode-switch,
  .scope {
    display: flex;
    align-items: center;
  }
  .broadcast-heading {
    min-height: 26px;
    gap: 12px;
  }
  .mode-switch {
    gap: 16px;
  }
  .scope {
    width: max-content;
    padding: 2px;
    border-radius: var(--radius-control);
  }
  .mode-switch button,
  .scope button,
  .close,
  .stop-live {
    border: 0;
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    cursor: default;
  }
  .mode-switch button {
    height: 26px;
    padding: 0 2px;
    border-radius: 0;
    color: var(--text-muted);
  }
  .mode-switch button.active {
    box-shadow: inset 0 -2px var(--text-secondary);
    color: var(--text-primary);
  }
  .scope button {
    height: 24px;
    padding: 0 9px;
    border-radius: 4px;
    white-space: nowrap;
  }
  .scope button.active {
    background: var(--selection-inactive);
    color: var(--text-primary);
  }
  .mode-switch button:hover,
  .scope button:hover,
  .close:hover,
  .stop-live:hover {
    background: var(--control-hover);
    color: var(--text-primary);
  }
  .close {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    margin-left: auto;
    border-radius: var(--radius-control);
  }
  .broadcast-form,
  .live-setup {
    display: grid;
    min-width: 0;
    width: 100%;
    overflow: visible;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    background: var(--surface-raised);
  }
  .broadcast-form:focus-within {
    border-color: var(--focus-ring);
  }
  .command {
    width: 100%;
    min-width: 0;
    min-height: 44px;
    max-height: 112px;
    padding: 7px 10px;
    resize: vertical;
    border: 0;
    border-radius: var(--radius-control) var(--radius-control) 0 0;
    background: transparent;
    color: var(--text-primary);
    font: var(--ui-font-body) ui-monospace, "SF Mono", Menlo, monospace;
    line-height: 1.35;
  }
  .command:focus-visible {
    outline: none;
  }
  .destination-row {
    gap: 8px;
    min-width: 0;
    min-height: 36px;
    padding: 3px 6px;
    border-top: 1px solid var(--border-subtle);
  }
  .recipient-picker {
    position: relative;
    flex: 1;
    min-width: 0;
  }
  .recipient-picker summary {
    display: flex;
    align-items: center;
    gap: 7px;
    height: 26px;
    min-width: 0;
    padding: 0 8px;
    border: 0;
    border-radius: var(--radius-control);
    background: transparent;
    color: var(--text-secondary);
    cursor: default;
    list-style: none;
  }
  .recipient-picker summary::-webkit-details-marker {
    display: none;
  }
  .recipient-picker summary:hover {
    background: var(--control-hover);
  }
  .recipient-picker summary:focus-visible,
  .broadcast-bar button:focus-visible {
    outline: 2px solid var(--focus-ring);
    outline-offset: 2px;
  }
  .recipient-count {
    min-width: 0;
    overflow: hidden;
    color: var(--text-primary);
    font-weight: 500;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .recipient-summary {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .recipient-picker :global(.picker-chevron) {
    color: var(--text-muted);
  }
  .recipient-menu {
    position: absolute;
    top: calc(100% + 4px);
    right: 0;
    z-index: 10;
    width: min(400px, max(100%, 260px));
    max-width: calc(100vw - 24px);
    max-height: 240px;
    padding: 6px;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background: var(--surface-raised);
    box-shadow: var(--shadow-panel);
  }
  .recipient-list {
    display: grid;
    gap: 2px;
  }
  .recipient-list label {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 32px;
    padding: 4px 8px;
    border-radius: 4px;
    color: var(--text-primary);
    white-space: nowrap;
  }
  .recipient-list label:hover {
    background: var(--control-hover);
  }
  .recipient-list label.unavailable {
    color: var(--text-muted);
  }
  .recipient-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .recipient-list input {
    width: 14px;
    height: 14px;
    margin: 0;
    accent-color: var(--accent);
  }
  .recipient-status,
  .no-recipients {
    color: var(--text-muted);
    font-size: var(--ui-font-tiny);
  }
  .no-recipients {
    padding: 8px;
  }
  .send,
  .start-live {
    flex: none;
    height: 30px;
    padding: 0 12px;
    border: 0;
    border-radius: var(--radius-control);
    background: var(--accent);
    color: var(--accent-foreground);
    font: inherit;
    font-weight: 500;
    white-space: nowrap;
  }
  .send:hover:not(:disabled),
  .start-live:hover:not(:disabled) {
    filter: brightness(1.08);
  }
  .send:disabled,
  .start-live:disabled {
    border: 1px solid var(--border);
    background: var(--surface-raised);
    color: var(--text-disabled);
  }
  .close:disabled,
  .scope button:disabled {
    opacity: 0.5;
  }
  .broadcast-status {
    display: flex;
    flex-wrap: wrap;
    gap: 3px 10px;
    max-height: 44px;
    overflow-y: auto;
    font-size: var(--ui-font-tiny);
    line-height: 1.35;
  }
  .reminder {
    color: var(--text-muted);
  }
  .command-caution {
    color: var(--status-warning);
    font-size: var(--ui-font-small);
  }
  .error {
    color: var(--status-error);
  }
  .live-warning {
    display: block;
    padding: 8px 10px;
    border-left: 2px solid var(--status-warning);
    border-radius: var(--radius-control) var(--radius-control) 0 0;
  }
  .live-warning strong {
    color: var(--status-warning);
    margin-right: 6px;
  }
  .live-warning span {
    color: var(--text-secondary);
  }
  .live-strip {
    min-height: 36px;
    gap: 12px;
    margin: -6px -12px;
    padding: 4px 12px;
    border-left: 2px solid var(--status-warning);
    background: var(--status-warning-surface);
  }
  .live-strip strong {
    color: var(--status-warning);
    letter-spacing: 0.02em;
    white-space: nowrap;
  }
  .live-strip span {
    flex: 1;
    color: var(--text-secondary);
  }
  .stop-live {
    flex: none;
    height: 28px;
    margin-left: auto;
    padding: 0 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    color: var(--text-primary);
  }
  @container (max-width: 600px) {
    .destination-row {
      display: grid;
      grid-template-columns: auto minmax(0, 1fr);
    }
    .recipient-picker {
      width: 100%;
    }
    .scope button {
      padding: 0 6px;
    }
    .send,
    .start-live {
      grid-column: 2;
      justify-self: end;
      padding: 0 8px;
    }
    .live-strip span {
      display: none;
    }
  }
</style>
