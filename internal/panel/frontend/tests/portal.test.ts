import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * Whoever moves a node out of Svelte's reach owns removing it.
 *
 * Svelte takes a component's nodes out of where it put them. A node moved
 * somewhere else is therefore not removed at all. The old Modal used to
 * reparent its `<dialog>` to the shell; the bits-ui Dialog.Portal handles
 * reparenting internally, so this rule now applies to any component that
 * still does manual reparenting.
 *
 * The rule is checked as source because the behaviour that breaks it needs a
 * real top layer, and the runtime here has no DOM at all. It is narrow on
 * purpose: it asks only that a component which reparents a node also takes it
 * away.
 */

const components = new URL('../src/lib/components/', import.meta.url);

describe('a component that reparents a node', () => {
  const movers = readdirSync(components)
    .filter((file) => file.endsWith('.svelte'))
    .map((file) => [file, readFileSync(new URL(file, components), 'utf8')] as const)
    .filter(([, source]) => /\.(?:append|appendChild|insertBefore|prepend)\(/u.test(source));

  it('leaves portal ownership to the headless primitives', () => {
    expect(movers).toEqual([]);
  });

  it.each(movers.map(([file]) => file))('takes the node away again in %s', (file) => {
    const source = movers.find(([name]) => name === file)?.[1] ?? '';
    expect(source).toMatch(/\.remove\(\)/u);
  });

  it.each(movers.map(([file]) => file))('closes a dialog it moved, in %s', (file) => {
    const source = movers.find(([name]) => name === file)?.[1] ?? '';
    if (!source.includes('<dialog')) return;
    const teardown = source.slice(source.indexOf('return () => {'));
    expect(teardown.indexOf('.close()')).toBeLessThan(teardown.indexOf('.remove()'));
  });
});
