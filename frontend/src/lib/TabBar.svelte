<script lang="ts">
  import { tick } from "svelte";
  import Bookmark from "@lucide/svelte/icons/bookmark";
  import Plus from "@lucide/svelte/icons/plus";
  import X from "@lucide/svelte/icons/x";
  import { maximumWorkspaceSessions, sessionDisplayLabel, type Session, type SessionStatus, type Tab } from "./tabs";

  let {
    tabs,
    sessions,
    displayLabels,
    activeId,
    newTabShortcut,
    tilingMode,
    onselect,
    onclose,
    onbookmark,
    onnew,
    ondrop,
  }: {
    tabs: Tab[];
    sessions: Session[];
    displayLabels: ReadonlyMap<number, string>;
    activeId: number | null;
    newTabShortcut: string;
    tilingMode: boolean;
    onselect: (id: number) => void;
    onclose: (id: number) => void;
    onbookmark: (id: number) => void;
    onnew: (shiftKey: boolean) => void;
    ondrop: (sourceTabId: number, targetTabId: number) => void;
  } = $props();

  type DropState = "valid" | "full" | null;

  let dragOrigin: { id: number; x: number; y: number; pointerId: number; element: HTMLElement } | null = null;
  let suppressClick = false;
  let draggingTabId = $state<number | null>(null);
  let dropTargetId = $state<number | null>(null);
  let dropState = $state<DropState>(null);
  let tabbar: HTMLDivElement;

  $effect(() => {
    const selectedId = activeId;
    void tick().then(() => {
      tabbar?.querySelector<HTMLElement>(`[data-tab-id="${selectedId}"]`)
        ?.scrollIntoView({ block: "nearest", inline: "nearest" });
    });
  });

  function sessionFor(id: number): Session | undefined {
    return sessions.find((session) => session.id === id);
  }

  function titleFor(tab: Tab): string {
    if (tab.kind === "session") {
      return sessionDisplayLabel(displayLabels, tab.sessionId);
    }
    return "Workspace";
  }

  function memberLabelsFor(tab: Tab): string[] {
    return tab.kind === "workspace" ? tab.sessionIds.map((id) => sessionDisplayLabel(displayLabels, id)) : [];
  }

  function accessibleTitleFor(tab: Tab): string {
    if (tab.kind === "session") {
      return titleFor(tab);
    }
    return `${titleFor(tab)}: ${memberLabelsFor(tab).join(" · ")}`;
  }

  function statusFor(tab: Tab): SessionStatus {
    const statuses = (tab.kind === "session" ? [tab.sessionId] : tab.sessionIds)
      .map((id) => sessionFor(id)?.status)
      .filter((status): status is SessionStatus => status !== undefined);
    if (statuses.includes("connecting")) {
      return "connecting";
    }
    if (statuses.includes("live")) {
      return "live";
    }
    return "closed";
  }

  function statusLabel(status: SessionStatus): string {
    switch (status) {
      case "connecting":
        return "Connecting";
      case "live":
        return "Live";
      case "closed":
        return "Closed";
    }
  }

  function stateFor(target: Tab): DropState {
    if (draggingTabId === null || target.id === draggingTabId) {
      return null;
    }
    const source = tabs.find((tab) => tab.id === draggingTabId);
    if (!source || source.kind !== "session") {
      return null;
    }
    if (target.kind === "workspace" && target.sessionIds.length >= maximumWorkspaceSessions) {
      return "full";
    }
    return "valid";
  }

  function dragStart(event: PointerEvent, tab: Tab) {
    suppressClick = false;
    if (event.button !== 0 || tab.kind !== "session" || (event.target as HTMLElement).closest("button")) {
      return;
    }
    event.preventDefault();
    const element = event.currentTarget as HTMLElement;
    dragOrigin = { id: tab.id, x: event.clientX, y: event.clientY, pointerId: event.pointerId, element };
    element.setPointerCapture(event.pointerId);
  }

  function dragMove(event: PointerEvent) {
    if (!dragOrigin || event.pointerId !== dragOrigin.pointerId) {
      return;
    }
    if (draggingTabId === null && Math.hypot(event.clientX-dragOrigin.x, event.clientY-dragOrigin.y) < 5) {
      return;
    }
    draggingTabId = dragOrigin.id;
    suppressClick = true;
    const element = document.elementFromPoint(event.clientX, event.clientY)?.closest<HTMLElement>("[data-tab-id]");
    const target = tabs.find((tab) => tab.id === Number(element?.dataset.tabId));
    dropTargetId = target?.id ?? null;
    dropState = target ? stateFor(target) : null;
  }

  function drop(event: PointerEvent) {
    dragMove(event);
    const sourceId = draggingTabId;
    const targetId = dropTargetId;
    const valid = dropState === "valid";
    clearDrag();
    if (valid && sourceId !== null && targetId !== null) {
      ondrop(sourceId, targetId);
    }
  }

  function clearDrag() {
    const origin = dragOrigin;
    dragOrigin = null;
    draggingTabId = null;
    dropTargetId = null;
    dropState = null;
    if (origin?.element.hasPointerCapture(origin.pointerId)) {
      origin.element.releasePointerCapture(origin.pointerId);
    }
  }

  function focusTab(index: number) {
    const tab = tabs[index];
    if (!tab) {
      return;
    }
    onselect(tab.id);
    requestAnimationFrame(() => {
      document.querySelector<HTMLElement>(`[data-tab-id="${tab.id}"]`)?.focus();
    });
  }

  function keydown(event: KeyboardEvent, index: number) {
    if (event.target !== event.currentTarget) {
      return;
    }
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onselect(tabs[index].id);
      return;
    }
    if (tabs.length === 0) {
      return;
    }
    let nextIndex: number | null = null;
    if (event.key === "ArrowRight") {
      nextIndex = (index + 1) % tabs.length;
    } else if (event.key === "ArrowLeft") {
      nextIndex = (index - 1 + tabs.length) % tabs.length;
    } else if (event.key === "Home") {
      nextIndex = 0;
    } else if (event.key === "End") {
      nextIndex = tabs.length - 1;
    }
    if (nextIndex !== null) {
      event.preventDefault();
      focusTab(nextIndex);
    }
  }
</script>

<svelte:window onblur={clearDrag} onkeydown={(event) => event.key === "Escape" && clearDrag()} />

<div bind:this={tabbar} class="tabbar" role="tablist">
  {#each tabs as tab (tab.id)}
    {@const status = statusFor(tab)}
    <div
      class="tab"
      class:active={tab.id === activeId}
      class:live={status === "live"}
      class:closed={status === "closed"}
      class:workspace={tab.kind === "workspace"}
      class:dragging={tab.id === draggingTabId}
      class:drop-valid={tab.id === dropTargetId && dropState === "valid"}
      class:drop-full={tab.id === dropTargetId && dropState === "full"}
      role="tab"
      data-tab-id={tab.id}
      tabindex={tab.id === activeId ? 0 : -1}
      aria-selected={tab.id === activeId}
      aria-label={`${accessibleTitleFor(tab)} — ${statusLabel(status)}`}
      title={`${accessibleTitleFor(tab)} — ${statusLabel(status)}`}
      onclick={() => { if (!suppressClick) onselect(tab.id); }}
      onkeydown={(event) => keydown(event, tabs.indexOf(tab))}
      onauxclick={(event) => event.button === 1 && onclose(tab.id)}
      onpointerdown={(event) => dragStart(event, tab)}
      onpointermove={dragMove}
      onpointerup={drop}
      onpointercancel={clearDrag}
      onlostpointercapture={clearDrag}
    >
      <span
        class="status"
        class:connecting={status === "connecting"}
        class:closed={status === "closed"}
        aria-hidden="true"
      ></span>
      <span class="title">{titleFor(tab)}</span>
      <span class="state-label">{statusLabel(status)}</span>
      {#if tab.kind === "workspace"}
        <span class="count" aria-label={`${tab.sessionIds.length} sessions`}>{tab.sessionIds.length}</span>
      {/if}
      {#if tab.kind === "session" && sessionFor(tab.sessionId)?.connection?.id === "" && tab.id === activeId}
        <button
          class="bookmark"
          aria-label="Save connection"
          title="Save connection"
          onclick={(event) => {
            event.stopPropagation();
            onbookmark(tab.sessionId);
          }}><Bookmark size={14} /></button
        >
      {/if}
      <button
        class="close"
        aria-label="Close tab"
        onclick={(event) => {
          event.stopPropagation();
          onclose(tab.id);
        }}><X size={14} /></button
      >
      {#if tab.id === dropTargetId && dropState !== null}
        <span class="drop-feedback">
          {dropState === "full" ? "Workspace full" : tab.kind === "workspace" ? "Add tile" : "Tile here"}
        </span>
      {/if}
    </div>
  {/each}
  <button
    class="new"
    aria-label="Open session"
    title={`Open session (${newTabShortcut}). Shift+click to open ${tilingMode ? "in a new tab" : "as a tile"}`}
    onclick={(event) => onnew(event.shiftKey)}
  ><Plus size={16} /></button>
</div>

<style>
  .tabbar {
    display: flex;
    flex: 1;
    min-width: 0;
    align-items: flex-end;
    gap: 2px;
    height: 40px;
    padding: 4px 6px 0;
    background: var(--toolbar);
    font: var(--ui-font-small) var(--font-ui);
    color: var(--toolbar-foreground);
    user-select: none;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .tabbar::-webkit-scrollbar {
    display: none;
  }
  .tab {
    /* Tabs drag to tile; only the empty strip around them drags the window. */
    --wails-draggable: no-drag;
    position: relative;
    display: flex;
    align-items: center;
    gap: 7px;
    height: 36px;
    padding: 0 5px 0 9px;
    min-width: 112px;
    max-width: 220px;
    border: 1px solid transparent;
    border-radius: var(--radius-control) var(--radius-control) 0 0;
    cursor: default;
  }
  .tab:not(.workspace) {
    cursor: grab;
    touch-action: none;
  }
  .tab.workspace {
    min-width: 150px;
  }
  .tab.active {
    background: var(--terminal-background);
    color: var(--text-primary);
  }
  .tab:focus-visible {
    outline: 2px solid var(--focus-ring);
    outline-offset: -2px;
  }
  .tab.dragging {
    opacity: 0.45;
  }
  .tab:hover:not(.active) {
    background: var(--toolbar-control-hover);
    color: var(--toolbar-control-foreground);
  }
  .tab.drop-valid,
  .tab.active.drop-valid {
    border-color: var(--accent);
    background: var(--accent-subtle);
  }
  .tab.drop-full,
  .tab.active.drop-full {
    border-color: var(--status-error);
    background: var(--status-error-surface);
  }
  .tab.closed .title {
    opacity: 0.5;
    text-decoration: line-through;
  }
  .state-label {
    flex: none;
    color: var(--text-muted);
    font-size: var(--ui-font-tiny);
  }
  .active .state-label {
    color: var(--text-secondary);
  }
  .tab.live:not(.active) .state-label {
    display: none;
  }
  .title {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .status {
    width: 6px;
    height: 6px;
    flex: none;
    border-radius: 50%;
    background: var(--text-secondary);
  }
  .status.connecting {
    border: 1px solid var(--text-secondary);
    background: transparent;
    animation: pulse 1.4s ease-in-out infinite;
  }
  .status.closed {
    border: 1px solid var(--text-muted);
    border-radius: 2px;
    background: transparent;
  }
  .count {
    display: grid;
    place-items: center;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    border-radius: 9px;
    background: var(--border-subtle);
    color: var(--text-secondary);
    font-size: var(--ui-font-micro);
  }
  .drop-feedback {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    border-radius: inherit;
    background: var(--selection);
    color: var(--text-primary);
    font-weight: 500;
    pointer-events: none;
  }
  .drop-full .drop-feedback {
    background: var(--status-error-surface);
    color: var(--status-error);
  }
  @keyframes pulse {
    50% { opacity: 0.35; }
  }
  @media (prefers-reduced-motion: reduce) {
    .status.connecting { animation: none; }
  }
  .close,
  .bookmark,
  .new {
    --wails-draggable: no-drag;
    display: grid;
    place-items: center;
    flex: none;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    width: 22px;
    height: 22px;
    border-radius: 6px;
    cursor: default;
  }
  .close {
    visibility: hidden;
  }
  .tab:hover .close,
  .tab.active .close {
    visibility: visible;
  }
  .close:hover,
  .bookmark:hover,
  .new:hover {
    background: var(--toolbar-control-hover);
    color: var(--text-primary);
  }
  .close:focus-visible,
  .bookmark:focus-visible,
  .new:focus-visible {
    outline: 2px solid var(--focus-ring);
    outline-offset: -2px;
  }
  .new {
    align-self: center;
    margin-left: 2px;
    flex: none;
    border: 1px solid transparent;
  }
</style>
