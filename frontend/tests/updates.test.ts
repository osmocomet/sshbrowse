import assert from "node:assert/strict";
import test from "node:test";
import { claimStartupUpdateCheck, githubReleaseURL, canCheckForUpdates } from "../src/lib/updates.ts";
import { loadPreferences, preferenceKeys, savePreference } from "../src/lib/storage.ts";

const day = 24 * 60 * 60 * 1000;

function storage() {
  const values = new Map<string, string>();
  return { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value); } };
}

test("startup checks default on, preserve opt-out, and limit attempts to once per day", () => {
  const preferences = storage();
  const now = 100 * day;
  assert.equal(loadPreferences(preferences).checkUpdatesOnStartup, true);
  assert.equal(claimStartupUpdateCheck(preferences, true, now), true);
  assert.equal(claimStartupUpdateCheck(preferences, true, now + 1), false);
  assert.equal(claimStartupUpdateCheck(preferences, true, now + day - 1), false);
  assert.equal(claimStartupUpdateCheck(preferences, true, now + day), true);

  savePreference(preferences, preferenceKeys.checkUpdatesOnStartup, "false");
  assert.equal(loadPreferences(preferences).checkUpdatesOnStartup, false);
  assert.equal(claimStartupUpdateCheck(preferences, false, now + 2 * day), false);
  assert.equal(preferences.getItem(preferenceKeys.lastStartupUpdateCheck), String(now + day));
});

test("invalid timestamps and backwards clock changes allow a fresh check", () => {
  for (const timestamp of ["invalid", "Infinity", "-1", String(200 * day)]) {
    const preferences = storage();
    preferences.setItem(preferenceKeys.lastStartupUpdateCheck, timestamp);
    assert.equal(claimStartupUpdateCheck(preferences, true, 100 * day), true);
  }
});

test("startup checks tolerate unavailable preference storage", () => {
  const preferences = { getItem() { throw new Error("unavailable"); }, setItem() { throw new Error("unavailable"); } };
  assert.equal(claimStartupUpdateCheck(preferences, true), true);
  assert.equal(claimStartupUpdateCheck(preferences, false), false);
});

test("release links only open this repository's GitHub release pages", () => {
  const releaseURL = "https://github.com/osmocomet/sshbrowse/releases/tag/v0.2.0";
  assert.equal(githubReleaseURL(releaseURL), releaseURL);
  for (const url of [undefined, "", "javascript:alert(1)", "https://example.invalid/releases/tag/v0.2.0", "https://github.com/other/repo/releases/tag/v0.2.0", "https://user@github.com/osmocomet/sshbrowse/releases/tag/v0.2.0"]) {
    assert.equal(githubReleaseURL(url), "");
  }
});

test("only supported installations and package-manager releases check for updates", () => {
  assert.equal(canCheckForUpdates("supported"), true);
  assert.equal(canCheckForUpdates("package-manager"), true);
  for (const availability of [undefined, "development", "unsupported", "unavailable"]) assert.equal(canCheckForUpdates(availability), false);
});
