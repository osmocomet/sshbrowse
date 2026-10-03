import assert from "node:assert/strict";
import test from "node:test";

import { terminalMinimumContrastRatio, terminalThemeFor } from "../src/lib/appearance.ts";

test("neutral terminal colors stay constant across interface themes", () => {
  const warm = terminalThemeFor("warm", "neutral");
  const fjord = terminalThemeFor("fjord", "neutral");
  assert.deepEqual(fjord, warm);
  assert.equal(warm.background, "#000000");
  assert.notEqual(terminalThemeFor("warm", "follow").background, warm.background);
  assert.equal(terminalThemeFor("fjord", "follow").background, "#0d191d");
});

test("terminal contrast keeps the high contrast protection", () => {
  assert.equal(terminalMinimumContrastRatio("contrast", "follow"), 7);
  assert.equal(terminalMinimumContrastRatio("contrast", "neutral"), 7);
  assert.equal(terminalMinimumContrastRatio("classic", "follow"), 1);
  assert.equal(terminalMinimumContrastRatio("classic", "neutral"), 4.5);
  assert.equal(terminalMinimumContrastRatio("moss", "follow"), 4.5);
  assert.equal(terminalThemeFor("moss", "follow").background, "#171d19");
  assert.equal(terminalThemeFor("moss", "follow").foreground, "#e6eadf");
});
