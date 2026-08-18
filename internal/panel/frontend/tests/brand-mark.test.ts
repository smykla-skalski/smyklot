import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * The wordmark is drawn in exactly one place.
 *
 * The invitation page used to carry its own: a served PNG at a different size, "Smyklot" in title
 * case, and a second line reading "Panel invitation". Nothing kept it in step with the sidebar it
 * was meant to match, and it drifted. `BrandMark` is now the only definition of the mark, so a
 * change to the type, the spacing or the asset reaches every surface at once.
 *
 * Checked as source because the runtime here has no DOM and no cascade.
 */

const components = new URL('../src/lib/components/', import.meta.url);

const sources = readdirSync(components)
  .filter((file) => file.endsWith('.svelte'))
  .map((file) => [file, readFileSync(new URL(file, components), 'utf8')] as const);

const read = (file: string): string => sources.find(([name]) => name === file)?.[1] ?? '';

describe('the brand mark', () => {
  it('is the only component holding the wordmark', () => {
    const holders = sources
      .filter(([, source]) => source.includes('class="mark-name"'))
      .map(([file]) => file);

    expect(holders).toEqual(['BrandMark.svelte']);
  });

  it('is the only component importing the halo', () => {
    // The rail names the same file in a comment, sizing its collapsed overlay against the halo's
    // drawn geometry, so this asks who *imports* it rather than who mentions it.
    const importers = sources
      .filter(([, source]) =>
        /import\s+\w+\s+from\s+\x27[^\x27]*smyklot-halo\.svg\x27/.test(source),
      )
      .map(([file]) => file);

    expect(importers).toEqual(['BrandMark.svelte']);
  });

  it('is what the sidebar and the pages outside the panel both render', () => {
    // `NightPage` is the shell the invitation and the error pages share, so it stands the mark
    // up for both of them.
    // `BrandRow` is the sidebar's top, split out of `IdentityBar`; the mark went with it.
    for (const file of ['BrandRow.svelte', 'NightPage.svelte']) {
      expect(read(file)).toMatch(/<BrandMark\b/u);
    }
  });

  it('is the page heading only in the sidebar', () => {
    // Two `h1`s on the invitation page would leave the reader guessing which one names it, so the
    // mark steps down there and the invitation's own title takes the heading.
    expect(read('BrandRow.svelte')).toMatch(/<BrandMark[^>]*\sheading\b/u);
    expect(read('InvitationPage.svelte')).not.toMatch(/<BrandMark[^>]*\sheading\b/u);
  });

  it('keeps the rail styling the rail owns, reaching into the child', () => {
    // The collapsed sidebar hides the copy, centres the mark and scales the icon under a press.
    // Those depend on `.collapsed`, which is on the sidebar itself, so they stay in
    // `IdentityBar` and now cross TWO component boundaries to land - through `BrandRow`
    // and into `BrandMark`.
    const rail = read('IdentityBar.svelte');
    for (const selector of [':global(.mark-icon)', ':global(.mark-copy)', ':global(.mark)']) {
      expect(rail).toContain(selector);
    }
  });
});
