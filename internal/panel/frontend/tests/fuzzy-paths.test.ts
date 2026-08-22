import { describe, expect, it } from 'vitest';

import { matchPath, matchPaths } from '../src/lib/fuzzy';

const PATHS = [
  '.github/workflows/test.yaml',
  '.github/workflows/sync.yaml',
  '.github/CODEOWNERS',
  '.github/dependabot.yml',
  'renovate.json',
  'README.md',
  'docs/releasing.md',
  'internal/panel/frontend/src/app.css',
  'internal/storage/sqlstore/store.go',
  'cmd/smyklot/main.go',
];

const best = (query: string): string => matchPaths(PATHS, query)[0].path;

describe('matchPath [Unit]', () => {
  it('refuses a query whose characters are not all there, in order', () => {
    expect(matchPath('renovate.json', 'zzz')).toBeNull();
    expect(matchPath('renovate.json', 'nosj')).toBeNull();
  });

  it('marks the characters it actually matched', () => {
    const match = matchPath('.github/workflows/sync.yaml', 'sync');

    expect(match).not.toBeNull();
    expect(match?.positions.map((at) => '.github/workflows/sync.yaml'[at]).join('')).toBe('sync');
  });

  it('slides the match into the file name rather than picking it out of the directories', () => {
    // Every one of these letters appears earlier in the path, in order.
    const match = matchPath('internal/storage/sqlstore/store.go', 'store');

    expect(match?.positions[0]).toBeGreaterThan(
      'internal/storage/sqlstore/store.go'.lastIndexOf('/'),
    );
  });

  it('ignores case in both directions', () => {
    expect(matchPath('README.md', 'readme')).not.toBeNull();
    expect(matchPath('docs/releasing.md', 'RELEAS')).not.toBeNull();
  });

  it('scores a run of consecutive characters above the same letters scattered', () => {
    const together = matchPath('sync.yaml', 'sync');
    const scattered = matchPath('some-yak-nice-crop', 'sync');

    expect(together?.score).toBeGreaterThan(scattered?.score ?? 0);
  });
});

describe('matchPaths [Unit]', () => {
  it('prefers a file name match when the query carries no separator', () => {
    expect(best('sync')).toBe('.github/workflows/sync.yaml');
    expect(best('main.go')).toBe('cmd/smyklot/main.go');
    expect(best('app.css')).toBe('internal/panel/frontend/src/app.css');
  });

  it('takes a query with a separator as a question about the whole path', () => {
    expect(best('gh/wf')).toMatch(/^\.github\/workflows\//);
    expect(best('panel/src')).toBe('internal/panel/frontend/src/app.css');
  });

  it('finds a file from its initials', () => {
    expect(best('cow')).toBe('.github/CODEOWNERS');
  });

  it('returns everything, in its own order, for an empty query', () => {
    expect(matchPaths(PATHS, '')).toHaveLength(PATHS.length);
    expect(matchPaths(PATHS, '')[0].path).toBe(PATHS[0]);
  });

  it('caps what it returns, because nobody reads past the tenth row', () => {
    const many = Array.from({ length: 400 }, (_, index) => `pkg/module-${index}/main.go`);

    expect(matchPaths(many, 'main')).toHaveLength(50);
    expect(matchPaths(many, 'main', 10)).toHaveLength(10);
  });

  it('answers a keystroke over a large index quickly enough to type through', () => {
    const many = Array.from(
      { length: 20_000 },
      (_, index) => `repo-${index % 40}/internal/panel/component-${index}.svelte`,
    );

    const started = performance.now();
    matchPaths(many, 'panelcomp');
    const spent = performance.now() - started;

    // Generous by an order of magnitude: this is a floor against an accidental
    // quadratic, not a benchmark.
    expect(spent).toBeLessThan(250);
  });

  /**
   * A path whose lowercase is longer than itself.
   *
   * `positions` index the folded string and are read back against the original,
   * so a fold that changes length makes every index past it point one place too
   * far. `'I'.toLowerCase()` is two code units, so one Turkish-named file in one
   * repository made a match near the end read `path[length]` - `undefined` -
   * and `undefined.toLowerCase()` threw inside `bonusAt`, which took the finder
   * down for the whole installation rather than for that one path.
   */
  describe('a path the case fold would lengthen', () => {
    it('does not throw', () => {
      expect(() => matchPath('İİİİa.md', 'a')).not.toThrow();
      expect(() => matchPaths(['İstanbul.md', 'README.md'], 'md')).not.toThrow();
    });

    it('marks the characters it says it marked', () => {
      const found = matchPath('İabc.md', 'b');

      expect(found?.positions.map((at) => 'İabc.md'[at])).toEqual(['b']);
    });
  });
});
