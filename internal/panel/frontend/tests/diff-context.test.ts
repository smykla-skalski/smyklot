import { describe, expect, it } from 'vitest';
import { unifiedDiff } from '../src/lib/code-tokens';
import { diffContext } from '../src/lib/diff-context';

describe('conflict diff context', () => {
  it('keeps all changes, exact literals and original line numbers across folded runs', () => {
    const before = Array.from({ length: 20 }, (_, index) => `line ${index + 1}`);
    const after = [...before];
    before[3] = '"id": 9007199254740992';
    after[3] = '"id": 9007199254740993';
    after.splice(15, 0, '"tiny": 1e-400');
    const lines = unifiedDiff(before.join('\n') + '\n', after.join('\n') + '\n');
    const rows = diffContext(lines, 1);
    const changes = rows.flatMap((row) =>
      row.kind === 'line' && row.line.op !== ' ' ? [row] : [],
    );
    expect(changes.map(({ before, after, line }) => [before, after, line.text])).toEqual([
      [4, null, '"id": 9007199254740992'],
      [null, 4, '"id": 9007199254740993'],
      [null, 16, '"tiny": 1e-400'],
    ]);
    expect(rows.filter((row) => row.kind === 'gap').map((row) => row.lines.length)).toEqual([
      2, 9, 4,
    ]);
    const expanded = rows.flatMap((row) => (row.kind === 'gap' ? row.lines : [row]));
    expect(expanded.map((row) => row.line)).toEqual(lines);
    expect(expanded.filter((row) => row.line.op !== '+').map((row) => row.line.text)).toEqual(
      before,
    );
    expect(expanded.filter((row) => row.line.op !== '-').map((row) => row.line.text)).toEqual(
      after,
    );
  });

  it('retains an unchanged file behind one revealable gap', () => {
    const rows = diffContext(unifiedDiff('a\nb\n', 'a\nb\n'), 2);
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({ kind: 'gap', start: 0 });
    if (rows[0]?.kind === 'gap')
      expect(rows[0].lines.map((row) => row.line.text)).toEqual(['a', 'b']);
  });

  it('keeps the full diff by default and supports zero context at either edge', () => {
    const lines = unifiedDiff('old\nmiddle\nlast', 'new\nmiddle\nend');
    expect(diffContext(lines).every((row) => row.kind === 'line')).toBe(true);
    const rows = diffContext(lines, 0);
    expect(rows.filter((row) => row.kind === 'line')).toHaveLength(4);
    expect(rows.find((row) => row.kind === 'gap')).toMatchObject({
      start: 2,
      lines: [{ line: { text: 'middle' } }],
    });
    expect(diffContext([], 2)).toEqual([]);
  });
});
