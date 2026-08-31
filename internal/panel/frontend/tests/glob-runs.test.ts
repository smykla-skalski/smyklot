import { describe, expect, it } from 'vitest';

import { globRuns } from '#lib/glob-runs.js';

/**
 * The design system states the rule in one line - "a ~TOKEN colours whole, a * colours
 * itself" - and every pattern in the product is drawn by it.
 */
describe('a pattern cut into what means and what is literal', () => {
  it('inks a token whole', () => {
    expect(globRuns('~DEFAULT_BRANCH')).toEqual([{ text: '~DEFAULT_BRANCH', meta: true }]);
  });

  it('inks a star and leaves the path around it', () => {
    expect(globRuns('release/*')).toEqual([
      { text: 'release/', meta: false },
      { text: '*', meta: true },
    ]);
  });

  it('reads a double star as the one token it is', () => {
    expect(globRuns('crates/**/target')).toEqual([
      { text: 'crates/', meta: false },
      { text: '**', meta: true },
      { text: '/target', meta: false },
    ]);
  });

  it('leaves a plain name alone', () => {
    expect(globRuns('main')).toEqual([{ text: 'main', meta: false }]);
  });

  it('has nothing to say about nothing', () => {
    expect(globRuns('')).toEqual([]);
  });
});
