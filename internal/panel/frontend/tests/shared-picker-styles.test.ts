import { describe, expect, it } from 'vitest';
import { componentSources, markupOf } from './support/markup';
describe('shared picker boundary', () => {
  it('leaves no native select menus in product components', () => {
    expect(
      componentSources()
        .filter(([, source]) => /<select[\s>]/u.test(markupOf(source)))
        .map(([file]) => file),
    ).toEqual([]);
  });
});
