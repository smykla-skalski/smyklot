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

  it('is imported only where the mark itself is drawn', () => {
    // This asks who *imports* the halo rather than who mentions it in a comment. Two sites: the
    // wordmark component, and the shell's rail, which opens with the bare mark the way the
    // approved shell does - a 34px icon with no wordmark beside it.
    const importers = sources
      .filter(([, source]) =>
        /import\s+\w+\s+from\s+\x27[^\x27]*smyklot-halo\.svg\x27/.test(source),
      )
      .map(([file]) => file);

    expect(importers).toEqual(['BrandMark.svelte', 'Rail.svelte']);
  });

  it('is what the pages outside the panel render', () => {
    // `NightPage` is the shell the invitation and the error pages share, so it stands the mark
    // up for both of them. Inside the panel the shell's mark is the rail's bare halo - an icon
    // with no wordmark - which the importer check above covers.
    expect(read('NightPage.svelte')).toMatch(/<BrandMark\b/u);
  });

  it('never takes the page heading from the page', () => {
    // Two `h1`s on the invitation page would leave the reader guessing which one names it, so the
    // mark steps down there and the invitation's own title takes the heading.
    expect(read('InvitationPage.svelte')).not.toMatch(/<BrandMark[^>]*\sheading\b/u);
  });
});
