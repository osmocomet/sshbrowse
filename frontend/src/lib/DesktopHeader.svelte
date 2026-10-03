<script lang="ts">
  import { onMount } from "svelte";
  import { Events, Window as WailsWindow } from "@wailsio/runtime";
  import { menuReturnFocusTarget, type EditMenuAction } from "./menuEvents";
  import type { TerminalFontCommand } from "./shortcuts";

  type MenuName = "app" | "shell" | "edit" | "view" | "window";
  type MenuEntry = {
    label?: string;
    shortcut?: string;
    separator?: boolean;
    run?: () => void;
  };

  let {
    showWindowControls,
    sidebarShortcut,
    onToggleSidebar,
    onToggleTiling,
    newTabShortcut,
    newLocalShortcut,
    newSFTPShortcut,
    newConnectionShortcut,
    closeTabShortcut,
    fontShortcuts,
    minimiseShortcut,
    quitShortcut,
    onnewtab,
    onnewlocal,
    onnewsftp,
    onnewconnection,
    onimport,
    onclosetab,
    onfont,
    oneditaction,
    onsettings,
    settingsActive,
    onabout,
    onupdate,
    onclosewindow,
    onminimise,
  }: {
    showWindowControls: boolean;
    sidebarShortcut: string;
    onToggleSidebar: (focusTarget?: HTMLElement) => void;
    onToggleTiling: () => void;
    newTabShortcut: string;
    newLocalShortcut: string;
    newSFTPShortcut: string;
    newConnectionShortcut: string;
    closeTabShortcut: string;
    fontShortcuts: {
      increase: string;
      decrease: string;
      reset: string;
    };
    minimiseShortcut: string;
    quitShortcut: string;
    onnewtab: () => void;
    onnewlocal: () => void;
    onnewsftp: () => void;
    onnewconnection: () => void;
    onimport: () => void;
    onclosetab: () => void;
    onfont: (command: TerminalFontCommand) => void;
    oneditaction: (action: EditMenuAction) => void;
    onsettings: () => void;
    settingsActive: boolean;
    onabout: () => void;
    onupdate: () => void;
    onclosewindow: () => void;
    onminimise: () => void;
  } = $props();

  let header: HTMLElement;
  let openMenu = $state<MenuName | null>(null);
  let activeMenuButton: HTMLButtonElement | null = null;
  let menuReturnFocus: HTMLElement | null = null;
  let lastExternalFocus: HTMLElement | null = null;
  let maximised = $state(false);
  let pendingAltMenu = false;

  function closeWindow() {
    onclosewindow();
  }

  function minimiseWindow() {
    onminimise();
  }

  function toggleMaximise() {
    WailsWindow.ToggleMaximise().catch((error) => console.error("Could not toggle SSHBrowse maximisation", error));
  }

  function toggleFullscreen() {
    WailsWindow.ToggleFullscreen().catch((error) => console.error("Could not toggle SSHBrowse fullscreen", error));
  }

  function menuEntries(name: MenuName): MenuEntry[] {
    switch (name) {
      case "app":
        return [
          { label: "About SSHBrowse", run: onabout },
          { label: "Check for Updates…", run: onupdate },
          { label: "Settings…", shortcut: "Ctrl+,", run: onsettings },
          { separator: true },
          { label: "Quit SSHBrowse", shortcut: quitShortcut, run: closeWindow },
        ];
      case "shell":
        return [
          { label: "New Tab…", shortcut: newTabShortcut, run: onnewtab },
          { label: "New Local Terminal", shortcut: newLocalShortcut, run: onnewlocal },
          { label: "New SFTP Tab", shortcut: newSFTPShortcut, run: onnewsftp },
          { label: "New Connection…", shortcut: newConnectionShortcut, run: onnewconnection },
          { label: "Import from SSH Config…", run: onimport },
          { separator: true },
          { label: "Close Tab", shortcut: closeTabShortcut, run: onclosetab },
        ];
      case "edit":
        return [
          { label: "Undo", shortcut: "Ctrl+Alt+Z", run: () => oneditaction("undo") },
          { label: "Redo", shortcut: "Ctrl+Alt+Shift+Z", run: () => oneditaction("redo") },
          { separator: true },
          { label: "Cut", shortcut: "Ctrl+Shift+X", run: () => oneditaction("cut") },
          { label: "Copy", shortcut: "Ctrl+Shift+C", run: () => oneditaction("copy") },
          { label: "Paste", shortcut: "Ctrl+Shift+V", run: () => oneditaction("paste") },
          { label: "Select All", shortcut: "Ctrl+Shift+A", run: () => oneditaction("selectAll") },
        ];
      case "view":
        return [
          { label: "Toggle Sidebar", shortcut: sidebarShortcut, run: () => onToggleSidebar() },
          { label: "Toggle Tiling Mode", run: onToggleTiling },
          { separator: true },
          { label: "Zoom In", shortcut: fontShortcuts.increase, run: () => onfont("increase") },
          { label: "Zoom Out", shortcut: fontShortcuts.decrease, run: () => onfont("decrease") },
          { label: "Reset Zoom", shortcut: fontShortcuts.reset, run: () => onfont("reset") },
          { separator: true },
          { label: "Full Screen", run: toggleFullscreen },
        ];
      case "window":
        return [
          { label: "Minimize", shortcut: minimiseShortcut, run: minimiseWindow },
          { label: maximised ? "Restore" : "Maximize", run: toggleMaximise },
          { separator: true },
          { label: "Close Window", shortcut: quitShortcut, run: closeWindow },
        ];
    }
  }

  function menuTriggers(): HTMLButtonElement[] {
    return Array.from(header?.querySelectorAll<HTMLButtonElement>("[data-menu-trigger]") ?? []);
  }

  function focusFirstItem(name: MenuName) {
    const panel = header?.querySelector<HTMLElement>(`[data-menu-panel="${name}"]`);
    panel?.querySelector<HTMLButtonElement>('button[role^="menuitem"]:not(:disabled)')?.focus();
  }

  function focusMenu(name: MenuName) {
    openMenu = name;
    requestAnimationFrame(() => focusFirstItem(name));
  }

  function openMenuFromTrigger(name: MenuName, trigger: HTMLButtonElement) {
    if (openMenu === name) {
      closeMenu();
      return;
    }
    if (openMenu === null) {
      const active = document.activeElement;
      menuReturnFocus = active instanceof HTMLElement && !header.contains(active)
        ? active
        : lastExternalFocus?.isConnected
          ? lastExternalFocus
          : trigger;
    }
    activeMenuButton = trigger;
    focusMenu(name);
  }

  function closeMenu(restoreFocus = true, preserveReturnFocus = false) {
    const returnFocus = menuReturnFocusTarget(menuReturnFocus, activeMenuButton);
    openMenu = null;
    activeMenuButton = null;
    if (!preserveReturnFocus) {
      menuReturnFocus = null;
    }
    if (restoreFocus && returnFocus?.isConnected) {
      returnFocus.focus();
    }
  }

  function activate(entry: MenuEntry) {
    closeMenu();
    entry.run?.();
  }

  function moveToMenuTrigger(trigger: HTMLButtonElement, delta: number, openNext = openMenu !== null) {
    const triggers = menuTriggers();
    const index = triggers.indexOf(trigger);
    if (index < 0 || triggers.length === 0) {
      return;
    }
    const next = triggers[(index + delta + triggers.length) % triggers.length];
    next.focus();
    if (openNext) {
      activeMenuButton = next;
      focusMenu(next.dataset.menuTrigger as MenuName);
    }
  }

  function handleTriggerKeydown(event: KeyboardEvent, name: MenuName) {
    const trigger = event.currentTarget as HTMLButtonElement;
    if (event.key === "Enter" || event.key === " " || event.key === "ArrowDown") {
      event.preventDefault();
      openMenuFromTrigger(name, trigger);
    } else if (event.key === "ArrowRight") {
      event.preventDefault();
      moveToMenuTrigger(trigger, 1);
    } else if (event.key === "ArrowLeft") {
      event.preventDefault();
      moveToMenuTrigger(trigger, -1);
    } else if (event.key === "Escape" && openMenu !== null) {
      event.preventDefault();
      closeMenu();
    }
  }

  function handleMenuKeydown(event: KeyboardEvent, name: MenuName) {
    if (event.key === "Tab") {
      // Leave focus to the browser's tab order; do not restore the action target.
      closeMenu(false);
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      closeMenu();
      return;
    }
    if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
      event.preventDefault();
      const delta = event.key === "ArrowRight" ? 1 : -1;
      const trigger = activeMenuButton;
      const shouldOpenNext = openMenu !== null;
      const returnFocus = menuReturnFocusTarget(menuReturnFocus, activeMenuButton);
      closeMenu(false, true);
      menuReturnFocus = returnFocus;
      if (trigger) {
        moveToMenuTrigger(trigger, delta, shouldOpenNext);
      }
      return;
    }

    const panel = header?.querySelector<HTMLElement>(`[data-menu-panel="${name}"]`);
    const items = Array.from(panel?.querySelectorAll<HTMLButtonElement>('button[role^="menuitem"]:not(:disabled)') ?? []);
    const current = items.indexOf(document.activeElement as HTMLButtonElement);
    if (items.length === 0) {
      return;
    }
    let next = current;
    if (event.key === "ArrowDown") {
      next = (current + 1 + items.length) % items.length;
    } else if (event.key === "ArrowUp") {
      next = (current - 1 + items.length) % items.length;
    } else if (event.key === "Home") {
      next = 0;
    } else if (event.key === "End") {
      next = items.length - 1;
    } else {
      return;
    }
    event.preventDefault();
    items[next]?.focus();
  }

  function handleChromeActivationKeydown(event: KeyboardEvent) {
    if (!showWindowControls) {
      if (event.key === "Alt" && !event.repeat && !event.ctrlKey && !event.metaKey) {
        pendingAltMenu = true;
      } else if (event.key !== "Alt") {
        pendingAltMenu = false;
      }
      if (event.key === "F10" && !event.ctrlKey && !event.altKey && !event.metaKey && !event.shiftKey) {
        event.preventDefault();
        event.stopPropagation();
        const first = menuTriggers()[0];
        if (first) openMenuFromTrigger("app", first);
        return;
      }
    }
  }

  function handleGlobalKeydown(event: KeyboardEvent) {
    if (event.key === "Escape" && openMenu !== null) {
      event.preventDefault();
      closeMenu();
    }
  }

  function handleGlobalKeyup(event: KeyboardEvent) {
    if (showWindowControls || event.key !== "Alt") return;
    const shouldOpen = pendingAltMenu && !event.ctrlKey && !event.metaKey;
    pendingAltMenu = false;
    if (!shouldOpen) return;
    event.preventDefault();
    event.stopPropagation();
    const first = menuTriggers()[0];
    if (first) openMenuFromTrigger("app", first);
  }

  function handleWindowBlur() {
    pendingAltMenu = false;
    if (openMenu !== null) closeMenu(false);
  }

  function handleGlobalFocusin(event: FocusEvent) {
    if (event.target instanceof HTMLElement && !header.contains(event.target)) {
      lastExternalFocus = event.target;
    }
  }

  function handleGlobalPointerdown(event: PointerEvent) {
    if (openMenu === null || !(event.target instanceof Element)) {
      return;
    }
    if (!header.contains(event.target) || event.target.closest("[data-menu-root]") === null) {
      // Let the clicked target receive focus after the menu is dismissed.
      closeMenu(false);
    }
  }

  onMount(() => {
    WailsWindow.IsMaximised()
      .then((value) => (maximised = value))
      .catch((error) => console.error("Could not read SSHBrowse maximisation state", error));
    const offMaximise = Events.On("common:WindowMaximise", () => (maximised = true));
    const offUnMaximise = Events.On("common:WindowUnMaximise", () => (maximised = false));
    return () => {
      offMaximise();
      offUnMaximise();
      closeMenu(false);
    };
  });
</script>

<svelte:window
  onkeydowncapture={handleChromeActivationKeydown}
  onkeydown={handleGlobalKeydown}
  onkeyupcapture={handleGlobalKeyup}
  onblur={handleWindowBlur}
  onfocusin={handleGlobalFocusin}
  onpointerdown={handleGlobalPointerdown}
/>

<header bind:this={header} class="desktop-window-header" class:windows-menu-header={!showWindowControls} role="menubar" aria-label="Application menu">
  <div class="menu-bar" inert={settingsActive}>
    {#each [
      ["app", "SSHBrowse"],
      ["shell", "Shell"],
      ...(!showWindowControls ? [["edit", "Edit"]] : []),
      ["view", "View"],
      ["window", "Window"],
    ] as [name, label]}
      {@const menuName = name as MenuName}
      <div class="menu-root" data-menu-root>
        <button
          class="menu-trigger"
          class:active={openMenu === menuName}
          type="button"
          role="menuitem"
          aria-haspopup="menu"
          aria-expanded={openMenu === menuName}
          data-menu-trigger={menuName}
          onclick={(event) => openMenuFromTrigger(menuName, event.currentTarget as HTMLButtonElement)}
          onpointerenter={(event) => {
            if (openMenu !== null && openMenu !== menuName) {
              openMenuFromTrigger(menuName, event.currentTarget as HTMLButtonElement);
            }
          }}
          onpointerdown={(event) => event.preventDefault()}
          onkeydown={(event) => handleTriggerKeydown(event, menuName)}
        >{label}</button>
        {#if openMenu === menuName}
          <div class="menu-panel" role="menu" tabindex="-1" aria-label={`${label} menu`} data-menu-panel={menuName} onkeydown={(event) => handleMenuKeydown(event, menuName)}>
            {#each menuEntries(menuName) as entry, index (entry.label ?? `separator-${index}`)}
              {#if entry.separator}
                <div class="menu-separator" role="separator"></div>
              {:else}
                <button
                  class="menu-entry"
                  type="button"
                  role="menuitem"
                  onclick={() => activate(entry)}
                >
                  <span>{entry.label}</span>
                  {#if entry.shortcut}<kbd>{entry.shortcut}</kbd>{/if}
                </button>
              {/if}
            {/each}
          </div>
        {/if}
      </div>
    {/each}
  </div>

  {#if showWindowControls}
    <div class="window-drag-space" aria-hidden="true" ondblclick={toggleMaximise}></div>

    <div class="window-controls" aria-label="Window controls">
    <button class="window-control" type="button" aria-label="Minimize window" title="Minimize window" onclick={minimiseWindow} onpointerdown={(event) => event.preventDefault()}>
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3 8h10" /></svg>
    </button>
    <button class="window-control" type="button" aria-label={maximised ? "Restore window" : "Maximize window"} title={maximised ? "Restore window" : "Maximize window"} onclick={toggleMaximise} onpointerdown={(event) => event.preventDefault()}>
      {#if maximised}
        <svg viewBox="0 0 16 16" aria-hidden="true"><rect x="4.5" y="4.5" width="7" height="7" /><path d="M4.5 6.5h-2v-3h6v1" /></svg>
      {:else}
        <svg viewBox="0 0 16 16" aria-hidden="true"><rect x="3.5" y="3.5" width="9" height="9" /></svg>
      {/if}
    </button>
    <button class="window-control close" type="button" aria-label="Close window" title="Close window" onclick={closeWindow} onpointerdown={(event) => event.preventDefault()}>
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8M12 4l-8 8" /></svg>
    </button>
    </div>
  {/if}
</header>

<style>
  .desktop-window-header {
    --wails-draggable: drag;
    display: flex;
    flex: none;
    align-items: stretch;
    min-width: 0;
    height: 46px;
    border-bottom: 1px solid var(--chrome-border);
    background: var(--chrome);
    color: var(--chrome-foreground);
    user-select: none;
  }
  .desktop-window-header.windows-menu-header {
    --wails-draggable: no-drag;
    height: 34px;
  }
  .menu-bar,
  .menu-root,
  .menu-trigger,
  .menu-panel,
  .menu-entry,
  .window-controls,
  .window-control {
    --wails-draggable: no-drag;
  }
  .menu-bar {
    display: flex;
    align-items: center;
    flex: none;
    gap: 3px;
    padding: 0 10px;
  }
  .menu-bar[inert] { opacity: 0.6; }
  .windows-menu-header .menu-bar {
    width: 100%;
  }
  .menu-root {
    position: relative;
    display: flex;
    align-items: stretch;
    height: 100%;
  }
  .menu-trigger {
    height: 34px;
    padding: 0 10px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--toolbar-foreground);
    cursor: default;
    font: var(--ui-font-body) var(--font-ui);
  }
  .menu-trigger:hover,
  .menu-trigger:focus-visible,
  .menu-trigger.active {
    background: var(--toolbar-control-hover);
    color: var(--toolbar-control-foreground);
  }
  .menu-trigger:focus-visible,
  .menu-entry:focus-visible,
  .window-control:focus-visible {
    outline: 2px solid var(--focus-ring);
    outline-offset: -2px;
  }
  .menu-panel {
    position: absolute;
    top: calc(100% - 4px);
    left: 0;
    z-index: 20;
    min-width: 214px;
    padding: 5px;
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background: var(--surface-overlay);
    box-shadow: var(--shadow-panel);
  }
  .menu-entry {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 24px;
    width: 100%;
    min-height: 32px;
    padding: 4px 8px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: var(--text-primary);
    cursor: default;
    font: var(--ui-font-body) var(--font-ui);
    text-align: left;
    white-space: nowrap;
  }
  .menu-entry:hover,
  .menu-entry:focus-visible {
    background: var(--control-hover);
  }
  .menu-entry kbd {
    color: var(--text-muted);
    font: var(--ui-font-tiny) var(--font-ui);
  }
  .menu-separator {
    height: 1px;
    margin: 4px 3px;
    background: var(--border-subtle);
  }
  .window-drag-space {
    --wails-draggable: drag;
    flex: 1 1 auto;
    min-width: 32px;
    cursor: default;
  }
  .window-controls {
    display: flex;
    flex: none;
    align-items: center;
    height: 46px;
    border-left: 1px solid var(--chrome-border);
  }
  .window-control {
    display: grid;
    width: 44px;
    height: 46px;
    place-items: center;
    border: 0;
    border-radius: 0;
    background: transparent;
    color: var(--toolbar-foreground);
    cursor: default;
  }
  .window-control:hover,
  .window-control:focus-visible {
    background: var(--toolbar-control-hover);
    color: var(--toolbar-control-foreground);
  }
  .window-control.close:hover,
  .window-control.close:focus-visible {
    background: var(--status-error-surface);
    color: var(--status-error);
  }
  .window-control svg {
    width: 14px;
    height: 14px;
    fill: none;
    stroke: currentColor;
    stroke-linecap: square;
    stroke-width: 1.25;
  }
</style>
