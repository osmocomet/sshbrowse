import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const stylesheet = readFileSync(new URL("../src/app.css", import.meta.url), "utf8");
const windowsChromeSource = readFileSync(new URL("../../internal/app/window.go", import.meta.url), "utf8");
const windowsChromeUpdatesSource = readFileSync(new URL("../../internal/app/window_theme_windows.go", import.meta.url), "utf8");
const themes = ["warm", "classic", "moss", "fjord", "oled", "contrast"] as const;

const sharedPrimitivePrefixes = ["--font-", "--ui-font-", "--radius-", "--shadow-"];

function paletteBody(theme: (typeof themes)[number]): string {
  const selector = theme === "classic"
    ? /:root\s*,\s*:root\[data-theme="classic"\]\s*,\s*\.preview\[data-theme="classic"\]\s*\{/
    : new RegExp(`:root\\[data-theme="${theme}"\\]\\s*,\\s*\\.preview\\[data-theme="${theme}"\\]\\s*\\{|\\.preview\\[data-theme="${theme}"\\]\\s*,\\s*:root\\[data-theme="${theme}"\\]\\s*\\{`);
  const matches = [...stylesheet.matchAll(new RegExp(selector.source, "g"))];

  assert.equal(matches.length, 1, `expected one ${theme} palette declaration`);

  const match = matches[0];
  assert.ok(match.index !== undefined);

  const bodyStart = match.index + match[0].length;
  const bodyEnd = stylesheet.indexOf("}", bodyStart);
  assert.notEqual(bodyEnd, -1, `expected a closing brace for the ${theme} palette`);

  return stylesheet.slice(bodyStart, bodyEnd);
}

function paletteRoles(theme: (typeof themes)[number]): Set<string> {
  const declarations = paletteBody(theme).matchAll(/^\s*(--[\w-]+)\s*:/gm);
  const roles = new Set<string>();

  for (const declaration of declarations) {
    const role = declaration[1];
    if (!sharedPrimitivePrefixes.some((prefix) => role.startsWith(prefix))) {
      roles.add(role);
    }
  }

  return roles;
}

test("each theme defines the full shared semantic color role set", () => {
  const warmRoles = [...paletteRoles("warm")].sort();

  for (const theme of themes.slice(1)) {
    assert.deepEqual([...paletteRoles(theme)].sort(), warmRoles, `${theme} palette roles should match Warm`);
  }
});

test("Windows title-bar colors match each interface palette", () => {
  const mapStart = windowsChromeSource.indexOf("var windowsChromePalettes =");
  assert.notEqual(mapStart, -1);
  const mapEnd = windowsChromeSource.indexOf("\n}", mapStart);
  assert.notEqual(mapEnd, -1);

  const entries = windowsChromeSource.slice(mapStart, mapEnd).matchAll(
    /^\s*"(\w+)":\s*\{(0x[0-9a-f]{6}),\s*(0x[0-9a-f]{6}),\s*(0x[0-9a-f]{6}),\s*(0x[0-9a-f]{6})\},?$/gm,
  );
  const chromePalettes = new Map([...entries].map((entry) => [entry[1], entry.slice(2).map(Number)]));
  assert.deepEqual([...chromePalettes.keys()].sort(), [...themes].sort());

  const roles = ["chrome", "chrome-foreground", "text-muted", "chrome-border"];
  for (const theme of themes) {
    const body = paletteBody(theme);
    const expected = roles.map((role) => {
      const color = body.match(new RegExp(`^\\s*--${role}:\\s*(#[0-9a-f]{6});`, "m"))?.[1];
      assert.ok(color, `${theme} is missing ${role}`);
      return Number(`0x${color.slice(5, 7)}${color.slice(3, 5)}${color.slice(1, 3)}`);
    });
    assert.deepEqual(chromePalettes.get(theme), expected, `${theme} Windows chrome differs from CSS`);
  }
});

test("the unthemed document starts with Classic colors", () => {
  const sharedRoot = stylesheet.match(/^:root \{([^}]+)\}/m)?.[1];
  assert.ok(sharedRoot, "shared interface tokens must be available before a theme is applied");
  assert.match(sharedRoot, /--font-ui: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;/);
  assert.match(sharedRoot, /--radius-control:/);
  assert.match(stylesheet, /:root\s*,\s*:root\[data-theme="classic"\]/);
  assert.match(windowsChromeSource, /func windowsChromeTheme\(\) application\.ThemeSettings \{\s*colours := windowsChromePalettes\["classic"\]/);
  assert.match(windowsChromeUpdatesSource, /selectedColours\.Store\(windowsChromePalettes\["classic"\]\)/);
});
