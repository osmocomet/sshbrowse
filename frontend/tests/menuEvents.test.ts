import assert from "node:assert/strict";
import test from "node:test";

import { menuReturnFocusTarget } from "../src/lib/menuEvents.ts";

test("menu navigation keeps the original action target instead of the trigger", () => {
  const terminalOrForm = {} as HTMLElement;
  const menuTrigger = {} as HTMLElement;

  assert.equal(menuReturnFocusTarget(terminalOrForm, menuTrigger), terminalOrForm);
  assert.equal(menuReturnFocusTarget(null, menuTrigger), menuTrigger);
});
