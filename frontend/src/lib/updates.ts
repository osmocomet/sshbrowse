import { preferenceKeys, savePreference, type PreferenceStorage } from "./storage.ts";

const startupCheckInterval = 24 * 60 * 60 * 1000;

export interface UpdateRelease {
  version: string;
  releaseURL: string;
}

export interface UpdateCheckResult extends UpdateRelease {
  automatic: boolean;
  checked: boolean;
  error: string;
}

export function canCheckForUpdates(availability: string | undefined): boolean {
  return availability === "supported" || availability === "package-manager";
}

// Limit attempts as well as successful checks: offline launches should not
// repeatedly contact GitHub. Manual checks do not use this timestamp.
export function claimStartupUpdateCheck(storage: PreferenceStorage, enabled: boolean, now = Date.now()): boolean {
  if (!enabled) return false;
  try {
    const lastCheck = Number(storage.getItem(preferenceKeys.lastStartupUpdateCheck));
    if (Number.isFinite(lastCheck) && lastCheck > 0 && lastCheck <= now && now - lastCheck < startupCheckInterval) {
      return false;
    }
    savePreference(storage, preferenceKeys.lastStartupUpdateCheck, String(now));
  } catch {
    // Restricted storage still permits a single check in this app instance.
  }
  return true;
}

export function githubReleaseURL(value: unknown): string {
  if (typeof value !== "string") return "";
  try {
    const url = new URL(value);
    return url.origin === "https://github.com" && url.pathname.startsWith("/osmocomet/sshbrowse/releases/tag/") && !url.username && !url.password
      ? url.href : "";
  } catch {
    return "";
  }
}
