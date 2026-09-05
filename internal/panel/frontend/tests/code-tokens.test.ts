import { describe, expect, it } from 'vitest';

import { emphasizeRuns, tokenizeLine, unifiedDiff, type TokenRun } from '../src/lib/code-tokens';

const text = (runs: TokenRun[]): string => runs.map((run) => run.text).join('');

describe('tokenizeLine', () => {
  it('cuts a JSON line into key, punctuation and string without losing a byte', () => {
    const line = '"schedule": ["* 4 * * 0"],';
    const runs = tokenizeLine('json', line);

    expect(text(runs)).toBe(line);
    expect(runs.find((run) => run.cls === 'tok-key')?.text).toBe('"schedule"');
    expect(runs.filter((run) => run.cls === 'tok-str').map((run) => run.text)).toContain(
      '"* 4 * * 0"',
    );
  });

  it('reads a YAML key and its constant', () => {
    const runs = tokenizeLine('yaml', 'retries: 3');

    expect(text(runs)).toBe('retries: 3');
    expect(runs.find((run) => run.cls === 'tok-key')?.text).toBe('retries');
    expect(runs.find((run) => run.cls === 'tok-const')?.text).toBe(' 3');
  });

  it('marks a markdown heading whole', () => {
    expect(tokenizeLine('markdown', '# Contributing')).toEqual([
      { cls: 'tok-head', text: '# Contributing' },
    ]);
  });
});

describe('unifiedDiff', () => {
  const before = [
    '"extends": ["config:recommended"],',
    '"schedule": ["* 4 * * 0"],',
    '"packageRules": [',
  ].join('\n');
  const after = [
    '"extends": ["config:recommended"],',
    '"schedule": ["* 4 * * 1-5"],',
    '"timezone": "Europe/Warsaw",',
    '"packageRules": [',
  ].join('\n');

  it('handles a large file without a quadratic diff table', () => {
    const lines = Array.from({ length: 20_000 }, (_, index) => `line ${index}`);
    const after = [...lines];
    after[10_000] = 'changed';
    const diff = unifiedDiff(lines.join('\n'), after.join('\n'));
    expect(diff.filter((line) => line.op !== ' ').map((line) => line.text)).toEqual([
      'line 10000',
      'changed',
    ]);
    expect(diff.filter((line) => line.op !== '-').map((line) => line.text)).toEqual(after);
    const replaced = unifiedDiff(lines.join('\n'), lines.map((line) => `new ${line}`).join('\n'));
    expect(replaced.filter((line) => line.op === '+')).toHaveLength(lines.length);
    expect(replaced.filter((line) => line.op === '-')).toHaveLength(lines.length);
  });

  it("renders the plan page's window: context, del, adds, context", () => {
    expect(unifiedDiff(before, after).map((line) => line.op)).toEqual([' ', '-', '+', '+', ' ']);
  });

  it('emphasises the changed stretch on both halves of a paired line', () => {
    const lines = unifiedDiff(before, after);
    const del = lines.find((line) => line.op === '-');
    const add = lines.find((line) => line.op === '+' && line.text.includes('schedule'));

    const stretch = (line: typeof del): string => {
      const [start, end] = line?.emphasis?.[0] ?? [0, 0];
      return (line?.text ?? '').slice(start, end);
    };

    expect(stretch(del)).toBe('0');
    expect(stretch(add)).toBe('1-5');
  });

  it('leaves an unpaired insertion without emphasis', () => {
    const timezone = unifiedDiff(before, after).find((line) => line.text.includes('timezone'));

    expect(timezone?.op).toBe('+');
    expect(timezone?.emphasis).toBeUndefined();
  });

  it('reads an empty before as pure insertion', () => {
    expect(unifiedDiff('', 'one\ntwo').map((line) => line.op)).toEqual(['+', '+']);
  });
});

describe('emphasizeRuns', () => {
  it('splits a run at the emphasis boundary and keeps its class', () => {
    const runs = emphasizeRuns([{ cls: 'tok-str', text: '"* 4 * * 1-5"' }], [[9, 12]]);

    expect(text(runs)).toBe('"* 4 * * 1-5"');
    expect(runs.filter((run) => run.word === true).map((run) => run.text)).toEqual(['1-5']);
    expect(runs.every((run) => run.cls === 'tok-str')).toBe(true);
  });
});
