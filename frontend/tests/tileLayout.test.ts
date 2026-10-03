import assert from "node:assert/strict";
import test from "node:test";

import { tileGrid } from "../src/lib/tileLayout.ts";

const expectedRowSizes: Record<number, number[]> = {
  1: [1],
  2: [2],
  3: [2, 1],
  4: [2, 2],
  5: [3, 2],
  6: [3, 3],
  7: [3, 2, 2],
  8: [3, 3, 2],
  9: [3, 3, 3],
};

test("tile layouts distribute one through nine sessions across balanced rows", () => {
  for (let count = 1; count <= 9; count += 1) {
    const layout = tileGrid(count);
    const actualRowSizes = Array.from({ length: layout.rows }, (_, row) =>
      layout.placements.filter((placement) => placement.row === row + 1).length,
    );

    assert.deepEqual(actualRowSizes, expectedRowSizes[count]);
    assert.equal(layout.placements.length, count);
  }
});

test("every tile row is fully covered without overlaps", () => {
  for (let count = 1; count <= 9; count += 1) {
    const layout = tileGrid(count);
    const occupiedTracks = new Set<string>();

    for (const placement of layout.placements) {
      assert.ok(placement.column >= 1);
      assert.ok(placement.column + placement.columnSpan <= layout.columns + 1);
      for (let column = placement.column; column < placement.column + placement.columnSpan; column += 1) {
        const track = `${placement.row}:${column}`;
        assert.equal(occupiedTracks.has(track), false, `overlap at ${track} for ${count} tiles`);
        occupiedTracks.add(track);
      }
    }

    assert.equal(occupiedTracks.size, layout.columns * layout.rows);
    for (let row = 1; row <= layout.rows; row += 1) {
      for (let column = 1; column <= layout.columns; column += 1) {
        assert.equal(occupiedTracks.has(`${row}:${column}`), true, `hole at ${row}:${column} for ${count} tiles`);
      }
    }
  }
});

test("placements follow logical workspace order", () => {
  const logicalSessionIds = [42, 7, 19];
  const layout = tileGrid(logicalSessionIds.length);

  assert.deepEqual(
    logicalSessionIds.map((sessionId, index) => ({ sessionId, placement: layout.placements[index] })),
    [
      { sessionId: 42, placement: { row: 1, column: 1, columnSpan: 1 } },
      { sessionId: 7, placement: { row: 1, column: 2, columnSpan: 1 } },
      { sessionId: 19, placement: { row: 2, column: 1, columnSpan: 2 } },
    ],
  );
});
