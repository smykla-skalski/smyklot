import { describe, expect, it } from 'vitest';

import { fuzzyCandidates } from '../src/lib/fuzzy';

const candidates = [
  { id: 'global', label: 'Global', keywords: ['all installations'] },
  { id: 'platform', label: 'Platform Engineering', keywords: ['kong-platform'] },
  { id: 'security', label: 'Security', keywords: ['sec-team'] },
];

describe('fuzzy candidate search', () => {
  it('prioritizes direct matches and keeps stable empty-query order', () => {
    expect(fuzzyCandidates(candidates, '').map(({ id }) => id)).toEqual([
      'global',
      'platform',
      'security',
    ]);
    expect(fuzzyCandidates(candidates, 'plat').map(({ id }) => id)).toEqual(['platform']);
  });

  it('matches subsequences, keywords, and diacritics', () => {
    expect(fuzzyCandidates(candidates, 'pfeng').map(({ id }) => id)).toEqual(['platform']);
    expect(fuzzyCandidates(candidates, 'all install').map(({ id }) => id)).toEqual(['global']);
    expect(fuzzyCandidates([{ id: 'one', label: 'Zażółć' }], 'zazolc').map(({ id }) => id)).toEqual(
      ['one'],
    );
  });
});
