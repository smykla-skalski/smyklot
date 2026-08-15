import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * An animation that ends a state cannot be squashed to nothing.
 *
 * `app.css` collapses every animation to 0.01ms under `prefers-reduced-motion: reduce`, with
 * `!important` on `*`. That is right for movement and wrong for a component that waits on
 * `animationend` to clear its own state: the animation there is not motion, it is how long
 * something stays on screen. Squashed, `CopyReceipt` removed the copy confirmation on the frame it
 * appeared, so a reduced-motion reader was told nothing at all.
 *
 * The rule is therefore: if a component's animation drives a state change, it must re-assert its
 * duration under the same media query, at a weight that survives the blanket rule. Dropping the
 * travel is the correct reduced-motion answer; dropping the time is not.
 *
 * Checked as source because the runtime here has no DOM and no cascade.
 */

const components = new URL('../src/components/', import.meta.url);

const gated = readdirSync(components)
  .filter((file) => file.endsWith('.svelte'))
  .map((file) => [file, readFileSync(new URL(file, components), 'utf8')] as const)
  .filter(([, source]) => /onanimationend=/u.test(source));

describe('a component whose animation ends a state', () => {
  it('finds the ones that do', () => {
    // If this drops to zero the rule below is checking nothing, which is worth knowing.
    expect(gated.length).toBeGreaterThan(0);
  });

  it.each(gated.map(([file]) => file))('survives the reduced-motion squash in %s', (file) => {
    const source = gated.find(([name]) => name === file)?.[1] ?? '';
    const reduced = source.slice(source.indexOf('prefers-reduced-motion'));
    expect(reduced).toMatch(/animation-duration:[^;]*!important/u);
  });
});
