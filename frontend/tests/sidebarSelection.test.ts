import assert from "node:assert/strict";
import test from "node:test";

import {
  emptyConnectionSelection,
  focusConnection,
  focusContextConnection,
  handleSidebarBackgroundFocus,
  selectAllVisible,
  selectVisibleConnection,
} from "../src/lib/sidebarSelection.ts";

const visibleIds = ["top", "alpha", "beta", "gamma"];

test("plain selection sets the active row and range anchor", () => {
  assert.deepEqual(
    selectVisibleConnection(emptyConnectionSelection(), "alpha", visibleIds, "replace"),
    { selectedIds: ["alpha"], anchorId: "alpha", activeId: "alpha" },
  );
});

test("shift selection extends from the anchor in either direction", () => {
  const single = selectVisibleConnection(emptyConnectionSelection(), "alpha", visibleIds, "replace");

  assert.deepEqual(selectVisibleConnection(single, "gamma", visibleIds, "extend"), {
    selectedIds: ["alpha", "beta", "gamma"],
    anchorId: "alpha",
    activeId: "gamma",
  });
  assert.deepEqual(selectVisibleConnection(single, "top", visibleIds, "extend"), {
    selectedIds: ["top", "alpha"],
    anchorId: "alpha",
    activeId: "top",
  });
});

test("shift-drag keeps its anchor while the pointer moves through visible rows", () => {
  const anchor = selectVisibleConnection(emptyConnectionSelection(), "alpha", visibleIds, "replace");
  const firstDragTarget = selectVisibleConnection(anchor, "beta", visibleIds, "extend");
  const secondDragTarget = selectVisibleConnection(firstDragTarget, "top", visibleIds, "extend");

  assert.deepEqual(firstDragTarget, {
    selectedIds: ["alpha", "beta"],
    anchorId: "alpha",
    activeId: "beta",
  });
  assert.deepEqual(secondDragTarget, {
    selectedIds: ["top", "alpha"],
    anchorId: "alpha",
    activeId: "top",
  });

  const collapsedAnchor = selectVisibleConnection(emptyConnectionSelection(), "alpha", ["top", "alpha", "gamma"], "replace");
  assert.deepEqual(selectVisibleConnection(collapsedAnchor, "gamma", ["top", "alpha", "gamma"], "extend"), {
    selectedIds: ["alpha", "gamma"],
    anchorId: "alpha",
    activeId: "gamma",
  });
});

test("command selection adds and removes rows without leaving a stale active row", () => {
  const alpha = selectVisibleConnection(emptyConnectionSelection(), "alpha", visibleIds, "replace");
  const added = selectVisibleConnection(alpha, "gamma", visibleIds, "toggle");

  assert.deepEqual(added, {
    selectedIds: ["alpha", "gamma"],
    anchorId: "gamma",
    activeId: "gamma",
  });
  assert.deepEqual(selectVisibleConnection(added, "gamma", visibleIds, "toggle"), {
    selectedIds: ["alpha"],
    anchorId: "alpha",
    activeId: "alpha",
  });
  assert.deepEqual(selectVisibleConnection(alpha, "alpha", visibleIds, "toggle"), emptyConnectionSelection());
});

test("a missing shift anchor starts a new selection", () => {
  const single = selectVisibleConnection(emptyConnectionSelection(), "alpha", visibleIds, "replace");

  assert.deepEqual(selectVisibleConnection(single, "gamma", ["top", "gamma"], "extend"), {
    selectedIds: ["gamma"],
    anchorId: "gamma",
    activeId: "gamma",
  });
});

test("select all retains a visible active row and handles an empty list", () => {
  const selected = { selectedIds: ["alpha"], anchorId: "alpha", activeId: "alpha" };

  assert.deepEqual(selectAllVisible(selected, visibleIds), {
    selectedIds: visibleIds,
    anchorId: "alpha",
    activeId: "alpha",
  });
  assert.deepEqual(selectAllVisible(selected, []), emptyConnectionSelection());
});

test("returning focus to a selected row preserves the multi-selection", () => {
  const selected = {
    selectedIds: ["alpha", "beta", "gamma"],
    anchorId: "alpha",
    activeId: "gamma",
  };

  assert.deepEqual(focusConnection(selected, "beta", visibleIds), {
    ...selected,
    activeId: "beta",
  });
  assert.deepEqual(focusConnection(selected, "top", visibleIds), {
    selectedIds: ["top"],
    anchorId: "top",
    activeId: "top",
  });

  assert.deepEqual(focusContextConnection(selected, "top", visibleIds), selected);
  assert.deepEqual(focusContextConnection(selected, "beta", visibleIds), {
    ...selected,
    activeId: "beta",
  });
});

test("background focus does not replace selection during context actions or dialog return", () => {
  const selected = {
    selectedIds: ["alpha", "beta"],
    anchorId: "alpha",
    activeId: "beta",
  };
  let current = selected;
  const background = {};
  const selectBackground = () => {
    current = emptyConnectionSelection();
  };
  const backgroundFocus = {
    target: background,
    currentTarget: background,
    relatedTarget: null,
  } as Pick<FocusEvent, "target" | "currentTarget" | "relatedTarget">;

  handleSidebarBackgroundFocus(backgroundFocus, true, false, selectBackground);
  assert.deepEqual(current, selected);
  handleSidebarBackgroundFocus(backgroundFocus, false, true, selectBackground);
  assert.deepEqual(current, selected);
  handleSidebarBackgroundFocus({
    ...backgroundFocus,
    relatedTarget: { closest: () => ({}) },
  } as Pick<FocusEvent, "target" | "currentTarget" | "relatedTarget">, false, false, selectBackground);
  assert.deepEqual(current, selected);

  handleSidebarBackgroundFocus(backgroundFocus, false, false, selectBackground);
  assert.deepEqual(current, emptyConnectionSelection());
});

test("background focus ignores bubbled row focus events", () => {
  let selected = false;
  const row = {};
  const background = {};

  handleSidebarBackgroundFocus({
    target: row,
    currentTarget: background,
    relatedTarget: null,
  } as Pick<FocusEvent, "target" | "currentTarget" | "relatedTarget">, false, false, () => {
    selected = true;
  });

  assert.equal(selected, false);
});
