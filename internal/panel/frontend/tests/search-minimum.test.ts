import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

import { SEARCH_MINIMUM } from '../src/lib/components/FindPalette.svelte';

function sourceOf(path: string): string {
  return readFileSync(resolve(process.cwd(), path), 'utf8');
}

/**
 * A search begins at two letters, in both places that offer one.
 *
 * One letter matches most of the panel - every page, every workspace, and whatever the
 * service answers for it - so it is a list nobody reads and a request nobody wanted. The
 * palette and the results page are one search, so they must begin at the same character:
 * a floor written twice is a floor that drifts, and a reader typing into the palette and
 * then pressing through to the page would be told two different things about their own
 * query.
 */
describe('the search floor [Unit]', () => {
  it('is two letters', () => {
    expect(SEARCH_MINIMUM).toBe(2);
  });

  it('is read from the one constant by both the palette and the page', () => {
    const palette = sourceOf('src/lib/components/FindPalette.svelte');
    const page = sourceOf('src/routes/search/+page.svelte');

    // Each gates its terms on the same expression, so nothing can search below it.
    expect(palette).toMatch(
      /const asking = \$derived\(query\.trim\(\)\.length >= SEARCH_MINIMUM\)/u,
    );
    expect(page).toMatch(/const asking = \$derived\(asked\.trim\(\)\.length >= SEARCH_MINIMUM\)/u);

    // And the page takes the number from the palette rather than writing its own.
    expect(page).toMatch(/SEARCH_MINIMUM.*from '#lib\/components\/FindPalette\.svelte'/u);
    expect(page).not.toMatch(/length >= 2\b/u);
  });

  it('spends no request on a query too short to answer', () => {
    const palette = sourceOf('src/lib/components/FindPalette.svelte');
    const page = sourceOf('src/routes/search/+page.svelte');

    // The lookup is the one thing here that costs the service anything.
    expect(palette).toMatch(/if \(lookup === undefined \|\| !asking\)/u);
    expect(page).toMatch(/if \(!asking \|\| lookup === undefined\)/u);
  });
});
