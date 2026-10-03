export interface TilePlacement {
  row: number;
  column: number;
  columnSpan: number;
}

export interface TileGrid {
  columns: number;
  rows: number;
  placements: TilePlacement[];
}

function greatestCommonDivisor(first: number, second: number): number {
  while (second !== 0) {
    const remainder = first % second;
    first = second;
    second = remainder;
  }
  return first;
}

function leastCommonMultiple(first: number, second: number): number {
  return (first / greatestCommonDivisor(first, second)) * second;
}

export function tileGrid(count: number): TileGrid {
  const normalizedCount = Math.max(0, Math.floor(count));
  if (normalizedCount === 0) {
    return { columns: 0, rows: 0, placements: [] };
  }

  const columnsPerRow = Math.ceil(Math.sqrt(normalizedCount));
  const rows = Math.ceil(normalizedCount / columnsPerRow);
  const rowSizes: number[] = [];
  const baseRowSize = Math.floor(normalizedCount / rows);
  const rowsWithExtraTile = normalizedCount % rows;
  for (let row = 0; row < rows; row += 1) {
    rowSizes.push(baseRowSize + (row < rowsWithExtraTile ? 1 : 0));
  }

  // A common track count lets each row fill the full width with equal panes.
  const columns = rowSizes.reduce(leastCommonMultiple, 1);
  const placements: TilePlacement[] = [];
  for (let row = 0; row < rows; row += 1) {
    const rowSize = rowSizes[row];
    const columnSpan = columns / rowSize;
    for (let column = 0; column < rowSize; column += 1) {
      placements.push({ row: row + 1, column: column * columnSpan + 1, columnSpan });
    }
  }

  return { columns, rows, placements };
}
