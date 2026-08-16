import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * Whoever moves a node out of Svelte's reach owns removing it.
 *
 * Svelte takes a component's nodes out of where it put them. A node moved somewhere else is
 * therefore not removed at all, and `Modal` moves its `<dialog>` to the shell so that a dialog
 * written inside something closed can still be seen - the inbox dialog lives in the account menu,
 * which is a closed popover most of the time. Without a teardown, dismissing a modal
 * that is conditionally rendered left the dialog in the document - still open, still in the top
 * layer - where it swallowed the first click on every other control on the page and looked for all
 * the world like the sidebar needing focus before it would answer.
 *
 * The rule is checked as source because the behaviour that breaks it needs a real top layer, and
 * the runtime here has no DOM at all. It is narrow on purpose: it asks only that a component which
 * reparents a node also takes it away.
 */

const components = new URL('../src/components/', import.meta.url);

describe('a component that reparents a node', () => {
  const movers = readdirSync(components)
    .filter((file) => file.endsWith('.svelte'))
    .map((file) => [file, readFileSync(new URL(file, components), 'utf8')] as const)
    .filter(([, source]) => /\.(?:append|appendChild|insertBefore|prepend)\(/u.test(source));

  it('finds the ones that do', () => {
    // If this drops to zero the rule below is checking nothing, which is worth knowing.
    expect(movers.length).toBeGreaterThan(0);
  });

  it.each(movers.map(([file]) => file))('takes the node away again in %s', (file) => {
    const source = movers.find(([name]) => name === file)?.[1] ?? '';
    expect(source).toMatch(/\.remove\(\)/u);
  });

  it.each(movers.map(([file]) => file))('closes a dialog it moved, in %s', (file) => {
    const source = movers.find(([name]) => name === file)?.[1] ?? '';
    if (!source.includes('<dialog')) return;
    // A dialog removed while open leaves the top layer holding a reference to it in some engines,
    // and a `.close()` after removal does not fire the events a caller may be waiting on.
    const teardown = source.slice(source.indexOf('return () => {'));
    expect(teardown.indexOf('.close()')).toBeLessThan(teardown.indexOf('.remove()'));
  });
});
