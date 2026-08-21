import { parse } from 'jsonc-parser';
import { describe, expect, it } from 'vitest';

import { mergedPreview } from '../src/lib/filemerge';
import { composeMergedText, deriveMerge } from '../src/lib/jsontext';

const TEMPLATE = [
  '{',
  '  "$schema": "https://docs.renovatebot.com/renovate-schema.json",',
  '  // Weekend runs keep review noise out of the working week',
  '  "extends": ["config:recommended"],',
  '  "schedule": ["* 4 * * 6"],',
  '  "timezone": "UTC",',
  '  "packageRules": [{ "matchDepTypes": ["devDependencies"], "automerge": true }],',
  '  "labels": ["dependencies"]',
  '}',
].join('\n');

describe('composeMergedText', () => {
  it('writes a changed key without touching the rest of the file', () => {
    const composed = composeMergedText(TEMPLATE, {
      strategy: 'deep-merge',
      overrides: { timezone: 'Europe/Warsaw' },
    });
    expect(composed).toContain('"timezone": "Europe/Warsaw"');
    // The comment and the compact one-line array survive - the re-print never did this.
    expect(composed).toContain('// Weekend runs keep review noise');
    expect(composed).toContain('"extends": ["config:recommended"]');
  });

  it('appends under a list rule, keeping the template entries first', () => {
    const composed = composeMergedText(TEMPLATE, {
      strategy: 'deep-merge',
      overrides: { labels: ['renovate'] },
      arrays: [{ path: 'labels', strategy: 'append' }],
    });
    expect(composed).not.toBeNull();
    const parsed = parse(composed ?? '') as { labels: string[] };
    expect(parsed.labels).toEqual(['dependencies', 'renovate']);
    // A compact list stays compact - the seam is the only new text.
    expect(composed).toContain('"labels": ["dependencies", "renovate"]');
  });

  it('an appended entry leaves the template entries their own bytes', () => {
    const composed = composeMergedText(TEMPLATE, {
      strategy: 'deep-merge',
      overrides: { packageRules: [{ matchManagers: ['npm'] }] },
      arrays: [{ path: 'packageRules', strategy: 'append' }],
    });
    // The template's own entry is untouched, so the gutter can mark only
    // what the adjustment added.
    expect(composed).toContain(
      '"packageRules": [{ "matchDepTypes": ["devDependencies"], "automerge": true }, { "matchManagers": ["npm"] }]',
    );
  });

  it('prepends into a multiline list without reflowing what stands', () => {
    const template = ['{', '  "list": [', '    { "a": 1 },', '    { "b": 2 }', '  ]', '}'].join(
      '\n',
    );
    const composed = composeMergedText(template, {
      strategy: 'deep-merge',
      overrides: { list: [{ z: 9 }] },
      arrays: [{ path: 'list', strategy: 'prepend' }],
    });
    expect(composed).toContain('    { "a": 1 },');
    expect(composed).toContain('    { "b": 2 }');
    expect(parse(composed ?? '')).toEqual({ list: [{ z: 9 }, { a: 1 }, { b: 2 }] });
  });

  it('agrees with the parsed-value merge on every strategy it speaks', () => {
    for (const strategy of ['deep-merge', 'shallow-merge']) {
      const merge = {
        strategy,
        overrides: {
          timezone: null,
          automerge: true,
          labels: ['renovate'],
          added: { nested: 1 },
        },
        arrays: [{ path: 'labels', strategy: 'prepend' }],
      };
      const text = composeMergedText(TEMPLATE, merge);
      const printed = mergedPreview(TEMPLATE, merge);
      expect(text, strategy).not.toBeNull();
      expect(parse(text ?? ''), strategy).toEqual(JSON.parse(printed ?? ''));
    }
  });

  it('declines what it cannot compose honestly', () => {
    expect(composeMergedText(TEMPLATE, { strategy: 'markdown' })).toBeNull();
    expect(composeMergedText('not json', { strategy: 'deep-merge' })).toBeNull();
  });
});

describe('deriveMerge', () => {
  it('round-trips: compose then derive gives the override back', () => {
    const merge = {
      strategy: 'deep-merge',
      overrides: { timezone: 'Europe/Warsaw', schedule: null },
    };
    const composed = composeMergedText(TEMPLATE, merge) ?? '';
    const derived = deriveMerge(TEMPLATE, composed, 'deep-merge', []);
    expect(derived?.overrides).toEqual({ timezone: 'Europe/Warsaw', schedule: null });
    expect(derived?.arrays).toEqual([]);
  });

  it('an unedited copy derives no override at all', () => {
    const derived = deriveMerge(TEMPLATE, TEMPLATE, 'deep-merge', []);
    expect(derived?.overrides).toEqual({});
    expect(derived?.questions).toEqual([]);
  });

  it('asks about a list that grew at the end, and answers change the override', () => {
    const edited = composeMergedText(TEMPLATE, {
      strategy: 'deep-merge',
      overrides: { labels: ['dependencies', 'renovate'] },
    });
    const asked = deriveMerge(TEMPLATE, edited ?? '', 'deep-merge', []);
    // Without an answer the merge does what it always does: replace.
    expect(asked?.overrides).toEqual({ labels: ['dependencies', 'renovate'] });
    expect(asked?.questions).toEqual([
      { path: 'labels', canAppend: true, canPrepend: false, chosen: 'replace' },
    ]);

    const answered = deriveMerge(TEMPLATE, edited ?? '', 'deep-merge', [
      { path: 'labels', strategy: 'append' },
    ]);
    expect(answered?.overrides).toEqual({ labels: ['renovate'] });
    expect(answered?.arrays).toEqual([{ path: 'labels', strategy: 'append' }]);
    expect(answered?.questions[0]?.chosen).toBe('append');
  });

  it('a reordered list is a replacement, with no question to ask', () => {
    const edited = composeMergedText(TEMPLATE, {
      strategy: 'deep-merge',
      overrides: { labels: ['renovate', 'dependencies-x'] },
    });
    const derived = deriveMerge(TEMPLATE, edited ?? '', 'deep-merge', [
      { path: 'labels', strategy: 'append' },
    ]);
    expect(derived?.overrides).toEqual({ labels: ['renovate', 'dependencies-x'] });
    expect(derived?.arrays).toEqual([]);
    expect(derived?.questions).toEqual([]);
  });

  it('derives nested changes as a nested patch under deep-merge', () => {
    const edited = TEMPLATE.replace('"automerge": true', '"automerge": false');
    const derived = deriveMerge(TEMPLATE, edited, 'deep-merge', []);
    // packageRules is a list of records - a changed entry replaces the list.
    expect(derived?.overrides).toHaveProperty('packageRules');
  });

  it('refuses what is not JSON rather than guessing', () => {
    expect(deriveMerge(TEMPLATE, '{ broken', 'deep-merge', [])).toBeNull();
    expect(deriveMerge(TEMPLATE, '[]', 'deep-merge', [])).toBeNull();
    expect(deriveMerge(TEMPLATE, '{}', 'markdown', [])).toBeNull();
  });
});
