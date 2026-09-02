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

  it('is drawn in two places only, each site taking its own cut', () => {
    // This asks who *addresses* the halo rather than who mentions it in a comment. Two sites and
    // two cuts of the same artwork, and which one goes where is the point: the rail wears the
    // BRAND cut - solid teal ring, interior painted - because a 34px badge on the sidebar's own
    // ground has to be an object. `BrandMark` wears the other, whose interior is transparent and
    // whose ring is the rainbow, because the invitation's night sky reads THROUGH the emblem: the
    // mark there is a window rather than a badge. Swapped, the rail shows a hole and the night
    // sky loses its sky.
    //
    // Addressed, not imported: both cuts are served from `static/`, because the favicon in
    // `app.html` needs a name it can write down and a bundled asset's carries a content hash.
    // That makes `static/` the one place either artwork lives - and importing out of it is the
    // thing that does not work, because Vite's dev server refuses to serve it.
    const drawn = sources
      .filter(([, source]) => /\$\{basePath\}\/smyklot-halo[\w-]*\.svg/u.test(source))
      .map(([file, source]) => [
        file,
        /smyklot-halo-brand\.svg/u.test(source) ? 'brand' : 'night-sky',
      ]);

    expect(drawn).toEqual([
      ['BrandMark.svelte', 'night-sky'],
      ['Rail.svelte', 'brand'],
    ]);
  });

  it('keeps both cuts where the favicon can reach them', () => {
    // The rule the line above depends on: one home for the artwork, and it is the one the
    // server publishes verbatim. A copy under `src/` would be a second artwork waiting to
    // drift from this one.
    const served = readdirSync(new URL('../static/', import.meta.url));

    expect(served).toContain('smyklot-halo.svg');
    expect(served).toContain('smyklot-halo-brand.svg');
    expect(readFileSync(new URL('../src/app.html', import.meta.url), 'utf8')).toContain(
      'smyklot-halo-brand.svg',
    );
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
