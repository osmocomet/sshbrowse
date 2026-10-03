import assert from "node:assert/strict";
import test from "node:test";

import { isComposingKey, shouldSaveOnEnter, type FormEnterTarget } from "../src/lib/formKeyboard.ts";

type EnterEvent = Pick<KeyboardEvent, "key" | "isComposing" | "keyCode">;

function enterEvent(overrides: Partial<EnterEvent> = {}): EnterEvent {
  return { key: "Enter", isComposing: false, keyCode: 13, ...overrides };
}

test("ordinary Enter in a text input requests the save-only path", () => {
  assert.equal(shouldSaveOnEnter(enterEvent(), { kind: "input", type: "text" }), true);
});

test("composition-confirming Enter is ignored, including WebKit keyCode 229", () => {
  const input: FormEnterTarget = { kind: "input", type: "text" };

  assert.equal(shouldSaveOnEnter(enterEvent({ isComposing: true }), input), false);
  assert.equal(shouldSaveOnEnter(enterEvent({ keyCode: 229 }), input), false);
});

test("composition keys are left to the input method", () => {
  assert.equal(isComposingKey(enterEvent()), false);
  assert.equal(isComposingKey(enterEvent({ isComposing: true })), true);
  assert.equal(isComposingKey(enterEvent({ keyCode: 229 })), true);
});

test("buttons, checkboxes, and textareas keep their native Enter behavior", () => {
  assert.equal(shouldSaveOnEnter(enterEvent(), { kind: "button" }), false);
  assert.equal(shouldSaveOnEnter(enterEvent(), { kind: "textarea" }), false);
  assert.equal(shouldSaveOnEnter(enterEvent(), { kind: "input", type: "checkbox" }), false);
});

test("Enter in a folder datalist keeps native suggestion handling", () => {
  assert.equal(shouldSaveOnEnter(enterEvent(), { kind: "input", type: "text", hasDatalist: true }), false);
});
