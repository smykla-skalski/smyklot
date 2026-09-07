import type { DiffLine } from './code-tokens';

export interface NumberedDiffLine {
  kind: 'line';
  index: number;
  before: number | null;
  after: number | null;
  line: DiffLine;
}

export interface DiffGap {
  kind: 'gap';
  start: number;
  lines: NumberedDiffLine[];
}

/** Fold only unchanged context, retaining source line numbers and every changed byte. */
export function diffContext(
  lines: readonly DiffLine[],
  contextLines?: number,
): Array<NumberedDiffLine | DiffGap> {
  let before = 0;
  let after = 0;
  const numbered: NumberedDiffLine[] = lines.map((line, index) => ({
    kind: 'line',
    index,
    before: line.op === '+' ? null : ++before,
    after: line.op === '-' ? null : ++after,
    line,
  }));
  if (contextLines === undefined || !Number.isFinite(contextLines)) return numbered;

  const context = Math.max(0, Math.floor(contextLines));
  const visible = new Uint8Array(lines.length);
  for (let index = 0; index < lines.length; index++) {
    if (lines[index]!.op === ' ') continue;
    visible.fill(1, Math.max(0, index - context), Math.min(lines.length, index + context + 1));
  }
  const result: Array<NumberedDiffLine | DiffGap> = [];
  for (let index = 0; index < numbered.length;) {
    if (visible[index]) {
      result.push(numbered[index++]!);
      continue;
    }
    const start = index;
    while (index < numbered.length && !visible[index]) index++;
    result.push({ kind: 'gap', start, lines: numbered.slice(start, index) });
  }
  return result;
}
