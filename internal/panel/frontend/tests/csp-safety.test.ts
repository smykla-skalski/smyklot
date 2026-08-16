import { describe, expect, it } from 'vitest';

// Comments explain the rule and quote the thing it forbids, so they are not markup.
import { componentSources, markupOf } from './support/markup';

/**
 * The panel serves `style-src 'self'`, under which a browser parses a `style` attribute written in
 * the markup and then throws it away.
 *
 * Nothing reports this. The attribute is still in the DOM to read back, the console says nothing
 * unless you are looking for it, and only the computed value shows that it never applied - so an
 * element sized through a custom property silently falls back to `auto`. It shipped that way:
 * `NightSky` lost both its dimensions and every `Avatar` in the panel lost its size, and the dev
 * server has no CSP so none of it showed up in development.
 *
 * Svelte's `style:` directive is the fix and the rule. It sets the property through
 * `element.style.setProperty`, which is script writing to the CSSOM rather than an inline style,
 * and CSP does not govern that. Verified against the built bundle: `setProperty(` appears in it
 * and `setAttribute("style"` does not.
 *
 * Checked as source, because the runtime here has no DOM, no cascade and no CSP.
 */

const sources = componentSources();

describe('styles the browser will actually apply', () => {
  it('has components to check', () => {
    expect(sources.length).toBeGreaterThan(20);
  });

  it.each(sources.map(([file]) => file))('sets no style attribute in %s', (file) => {
    const markup = markupOf(sources.find(([name]) => name === file)?.[1] ?? '');
    const offenders = [...markup.matchAll(/\sstyle=["']/gu)];

    expect(
      offenders,
      `${file} writes a style attribute; use the style: directive, which CSP allows`,
    ).toHaveLength(0);
  });

  it('still sizes the things that depend on a custom property', () => {
    // The two that broke. If either stops declaring its size through the directive, the rule above
    // is satisfied and the bug is back.
    const read = (file: string): string => sources.find(([name]) => name === file)?.[1] ?? '';

    expect(read('NightSky.svelte')).toMatch(/style:--sky-width=/u);
    expect(read('NightSky.svelte')).toMatch(/style:--sky-height=/u);
    expect(read('Avatar.svelte')).toMatch(/style:--avatar-size=/u);
  });
});
