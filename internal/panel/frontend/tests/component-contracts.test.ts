import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import { contractOf } from '../.storybook/contracts.js';

/**
 * Every component says what it is for, in the file it is written in.
 *
 * The contract is a `<!-- @component -->` block. Svelte defines it, an editor shows
 * it on hover, and `.storybook/contracts.ts` reads the same blocks into the
 * catalogue - so what a person reads on the Docs page and what the next person reads
 * while editing are the same sentence, and there is no second copy to go stale.
 *
 * `PENDING` is a ratchet, in the same shape as `story-coverage.test.ts` beside it: a
 * name may only ever leave it. A listed component that HAS gained a contract fails,
 * so the list cannot go stale, and one that is neither contracted nor listed fails,
 * so a new component cannot arrive undocumented.
 *
 * What belongs in a contract is what a call site cannot see: which of several
 * near-neighbours to reach for, what the component promises about when its effect
 * lands, and the rule that stops somebody re-deriving it wrongly. Not the prop list -
 * the props carry their own JSDoc, and the catalogue tabulates it.
 */
const components = fileURLToPath(new URL('../src/lib/components/', import.meta.url));

/**
 * Components still owed a contract. Only ever gets shorter.
 *
 * Empty, and that is the state to hold. All 105 carry one: 41 already had the
 * description as a JSDoc somewhere in the script and moved whole, and 64 were written
 * against what the component actually does.
 *
 * A name may be ADDED here only with a reason, and the bar is what a contract is for.
 * It is not a summary of the props - the props carry their own JSDoc and the catalogue
 * tabulates it - but the three things a call site cannot see: which of several
 * near-neighbours to reach for, what the component promises about when its effect
 * lands, and the rule that stops the next person re-deriving it wrongly.
 */
const PENDING: readonly string[] = [];

const names = readdirSync(components)
  .filter((file) => file.endsWith('.svelte'))
  .map((file) => file.replace('.svelte', ''))
  .sort();

const contracted = new Set(
  names.filter(
    (name) => contractOf(readFileSync(`${components}${name}.svelte`, 'utf8')) !== undefined,
  ),
);

describe('the component contracts [Unit]', () => {
  it('has components to contract', () => {
    // A guard that stands down when it cannot run is not a guard.
    expect(names.length).toBeGreaterThan(20);
  });

  const covered = names.filter((name) => !PENDING.includes(name));

  it.each(covered)('has a contract for %s', (name) => {
    expect(contracted.has(name), `${name}.svelte has no <!-- @component --> block`).toBe(true);
  });

  it('names every component it has not contracted yet', () => {
    expect(names.filter((name) => !contracted.has(name)).sort()).toEqual([...PENDING].sort());
  });

  it('lists nothing that no longer exists', () => {
    const gone = PENDING.filter((name) => !names.includes(name));
    expect(gone, 'remove these from PENDING, they are not components any more').toEqual([]);
  });

  it('writes a contract as prose rather than a heading or a prop list', () => {
    // The block is rendered as markdown at the head of a Docs page. A contract that
    // opens with a heading fights the component name already printed above it, and one
    // that lists props restates the table underneath it.
    const wrong = [...contracted].filter((name) => {
      const contract = contractOf(readFileSync(`${components}${name}.svelte`, 'utf8')) ?? '';
      return contract.startsWith('#') || /^\s*[-*]\s/mu.test(contract.split('\n')[0] ?? '');
    });
    expect(wrong).toEqual([]);
  });
});
