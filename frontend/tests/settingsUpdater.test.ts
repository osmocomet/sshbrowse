import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";
import { pathToFileURL } from "node:url";

import { compile } from "svelte/compiler";
import { render } from "svelte/server";

test("Settings shows check, download, retry, and restart controls for each update state", async () => {
  const source = readFileSync(new URL("../src/lib/SettingsPage.svelte", import.meta.url), "utf8");
  const compiled = compile(source, { filename: "SettingsPage.svelte", generate: "server" }).js.code;
  // Icons do not affect the update controls; replace them so Node can render
  // this component without a loader for Lucide's .svelte files.
  const code = compiled.replace(/^import (ArrowLeft|Check|X) from .*;$/gm, "const $1 = () => {};");
  const directory = mkdtempSync(join(process.cwd(), "node_modules/.settings-updater-test-"));
  try {
    const path = join(directory, "SettingsPage.mjs");
    writeFileSync(path, code);
    const { default: SettingsPage } = await import(pathToFileURL(path).href);
    const props = {
      themeName: "classic",
      terminalColors: "neutral",
      uiSize: "normal",
      interfaceScale: 1,
      terminalFontSize: 14,
      terminalFontName: "default",
      rightClickToPaste: false,
      copyOnSelection: false,
      sidebarWidth: 240,
      updateInfo: { version: "1.0.0", availability: "supported", message: "" },
      updateStatus: "",
      updateError: false,
      updateBusy: false,
      updateAvailable: false,
      updateReady: false,
    };

    const initial = render(SettingsPage, { props }).body;
    assert.match(initial, /Check for updates/);
    assert.doesNotMatch(initial, /Download update|Restart to update/);

    const current = render(SettingsPage, { props: { ...props, updateStatus: "SSHBrowse is up to date." } }).body;
    assert.match(current, /SSHBrowse is up to date\./);
    assert.match(current, /Check for updates/);

    const available = render(SettingsPage, { props: { ...props, updateAvailable: true, updateStatus: "Version 0.5.1 is available." } }).body;
    assert.match(available, /Version 0\.5\.1 is available\./);
    assert.match(available, /Download update/);
    assert.doesNotMatch(available, /Check for updates|Restart to update/);

    const ready = render(SettingsPage, { props: { ...props, updateReady: true, updateStatus: "Update ready. Restart to use the new version." } }).body;
    assert.match(ready, /Restart to update/);
    assert.match(ready, /Update ready\. Restart to use the new version\./);
    assert.doesNotMatch(ready, /Check for updates|Download update/);

    const failed = render(SettingsPage, { props: { ...props, updateError: true, updateStatus: "Update failed: network unavailable" } }).body;
    assert.match(failed, /role="alert"[^>]*>Update failed: network unavailable/);
    assert.match(failed, /Check for updates/);

    for (const availability of ["development", "package-manager"] as const) {
      const message = availability === "development"
        ? "In-app updates are unavailable for development builds."
        : "Updates for DEB and RPM installations are handled by your package manager.";
      const unsupported = render(SettingsPage, { props: { ...props, updateInfo: { version: "1.0.0", availability, message } } }).body;
      assert.ok(unsupported.includes(message));
      assert.doesNotMatch(unsupported, /Check for updates|Download update|Restart to update/);
    }
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
