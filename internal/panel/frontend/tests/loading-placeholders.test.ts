import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * A placeholder stands in for content that is not there. It never replaces content that is.
 *
 * Every one of these components reloads - a retry, a refresh event, a changed filter. When the
 * placeholder is gated on the request alone, a reload tears down what the reader is looking at and
 * puts back something of a different height, then undoes it a moment later. On the invitation page
 * that moved the whole card 45px up and back on every press of "Try again"; in the Root
 * installation view it blanked the entire panel on each refresh.
 *
 * So a loading branch has to name what is missing, not just that a request is in flight. The
 * in-place refresh is reported with `aria-busy` and a progress cursor instead, which is what the
 * tables in this panel already do.
 *
 * Checked as source because the runtime here has no DOM.
 */

const components = new URL('../src/components/', import.meta.url);

const sources = [
  ['App.svelte', readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')] as const,
  ...readdirSync(components)
    .filter((file) => file.endsWith('.svelte'))
    .map((file) => [file, readFileSync(new URL(file, components), 'utf8')] as const),
];

/** `{#if loading}` and `{:else if loadingUsers}` - a branch keyed on a request flag. */
const BRANCH = /\{[#:](?:else )?if ([^}]+)\}/gu;
const FLAG = /^\s*!?\s*(?:loading|busy|pending|fetching)\w*\s*$/iu;

const bareBranches = sources.flatMap(([file, source]) =>
  [...source.matchAll(BRANCH)]
    .map((match) => match[1] ?? '')
    .filter((expression) => FLAG.test(expression))
    .map((expression) => `${file}: {#if ${expression}}`),
);

describe('a loading placeholder', () => {
  it('never stands in front of content that already loaded', () => {
    // Qualify the branch with what is missing - `loading && page === null` - rather than widening
    // this list. Every component here can reload while showing something.
    expect(bareBranches).toEqual([]);
  });

  it('leaves the reader something to look at while a retry runs', () => {
    // Both of these clear their failure only once they have a replacement for it, so pressing
    // "Try again" keeps the message on screen instead of flashing the placeholder back.
    for (const file of ['InvitationPage.svelte', 'RootInstallationView.svelte']) {
      const source = sources.find(([name]) => name === file)?.[1] ?? '';
      const load = source.slice(source.indexOf('loading = true'), source.indexOf('} finally {'));

      expect(load, `${file} clears its failure before it has an answer`).not.toMatch(
        /loading = true;\s*\n\s*failure = null;/u,
      );
    }
  });
});
