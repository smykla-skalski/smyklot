import { describe, expect, it } from 'vitest';
import { componentSources, markupOf } from './support/markup';
describe('shared picker boundary', () => {
  it('gives editing-session Cancel buttons a visible border', () => {
    expect(
      componentSources()
        .filter(([, source]) =>
          /<Button\b[^>]*\btone=['"](?:quiet|ghost)['"][^>]*>\s*Cancel\s*<\/Button>/u.test(
            markupOf(source),
          ),
        )
        .map(([file]) => file),
    ).toEqual([]);
  });

  it('keeps optional add controls out of page-local chip styles', () => {
    expect(
      componentSources()
        .filter(([, source]) => /\badd-chip\b/u.test(source))
        .map(([file]) => file),
    ).toEqual([]);
  });

  it('leaves no native select menus in product components', () => {
    expect(
      componentSources()
        .filter(([, source]) => /<select[\s>]/u.test(markupOf(source)))
        .map(([file]) => file),
    ).toEqual([]);
  });
});
