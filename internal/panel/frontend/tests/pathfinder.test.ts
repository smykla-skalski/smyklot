import { describe, expect, it } from 'vitest';

import { fuzzyPath, rankPaths } from '../src/lib/pathfinder';
import { arrayRulePath, mergedPreview, mergeSummary } from '../src/lib/filemerge';

describe('the path finder [Unit]', () => {
  const KNOWN = [
    { path: '.github/workflows/ci.yaml', repositories: 25 },
    { path: '.github/workflows/release.yaml', repositories: 18 },
    { path: 'renovate.json', repositories: 21 },
    { path: 'CONTRIBUTING.md', repositories: 24 },
    { path: 'LICENSE', repositories: 25 },
  ];

  it('matches in-order subsequences and refuses everything else', () => {
    expect(fuzzyPath('wfci', '.github/workflows/ci.yaml')).not.toBeNull();
    expect(fuzzyPath('icfw', '.github/workflows/ci.yaml')).toBeNull();
  });

  it('reports the matched offsets for the marks', () => {
    const match = fuzzyPath('ren', 'renovate.json');
    expect(match?.positions).toEqual([0, 1, 2]);
  });

  it('prefers boundary starts over mid-word hits', () => {
    const ranked = rankPaths(KNOWN, 'ci');
    expect(ranked[0]?.path).toBe('.github/workflows/ci.yaml');
  });

  it('ranks the popular paths first while the query is empty', () => {
    const ranked = rankPaths(KNOWN, '');
    expect(ranked[0]?.repositories).toBe(25);
  });
});

describe('the merge preview [Unit]', () => {
  const TEMPLATE = JSON.stringify(
    {
      schedule: ['* 4 * * 6'],
      timezone: 'UTC',
      packageRules: [{ groupName: 'go modules' }],
      automerge: false,
    },
    null,
    2,
  );

  it('composes a deep merge with an append rule, the way the service does', () => {
    const preview = mergedPreview(TEMPLATE, {
      strategy: 'deep-merge',
      overrides: {
        timezone: 'Europe/Warsaw',
        packageRules: [{ groupName: 'frontend' }],
      },
      arrays: [{ path: '$.packageRules', strategy: 'append' }],
    });
    const parsed = JSON.parse(preview ?? '{}') as Record<string, unknown>;

    expect(parsed.timezone).toBe('Europe/Warsaw');
    expect(parsed.packageRules).toEqual([{ groupName: 'go modules' }, { groupName: 'frontend' }]);
    expect(parsed.automerge).toBe(false);
  });

  it('escapes every character the service path parser reserves', () => {
    expect(arrayRulePath(['host.rules', 'foo[bar\\baz'])).toBe(
      String.raw`$.host\.rules.foo\[bar\\baz`,
    );
  });

  it('removes a key for null, never writing it as null', () => {
    const preview = mergedPreview(TEMPLATE, { overrides: { automerge: null } });
    expect(Object.keys(JSON.parse(preview ?? '{}') as object)).not.toContain('automerge');
  });

  it('refuses to fake a preview of a strategy it does not speak', () => {
    expect(mergedPreview('# heading', { strategy: 'markdown' })).toBeNull();
  });

  it('splits an adjustment into the words the page says', () => {
    const summary = mergeSummary({
      overrides: {
        schedule: ['* 4 * * 1-5'],
        timezone: 'Europe/Warsaw',
        packageRules: [{ groupName: 'frontend' }],
        automerge: null,
      },
      arrays: [{ path: '$.packageRules', strategy: 'append' }],
    });

    /* A replaced list is a changed key; only a ruled one is "listed". */
    expect(summary.changed.sort()).toEqual(['schedule', 'timezone']);
    expect(summary.listed).toEqual([{ key: 'packageRules', strategy: 'append', entries: 1 }]);
    expect(summary.removed).toEqual(['automerge']);
  });
});
