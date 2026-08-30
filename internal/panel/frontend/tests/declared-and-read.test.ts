import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import { tokensStylesheet } from './theme';

/**
 * EVERY TOKEN THIS SHEET DECLARES IS READ SOMEWHERE.
 *
 * A token nobody reads is not a decision, it is a note about one - and it reads exactly like a
 * decision to whoever opens the file next. This sweep found forty-six of them at once: six that
 * described a borrowed hairline nothing borrowed, ten that named a page rhythm every view then
 * invented for itself, a whole tab vocabulary for a strip that had been retired, and a pair entry's
 * complete geometry for an entry the panel does not have. Each one had a paragraph above it
 * explaining a law, and not one of those laws was in force.
 *
 * There are only ever two honest answers. WIRE IT, at the distance or the state or the size the
 * token names - or DELETE IT, and say in the commit why this app does not have that thing. What is
 * not an answer is leaving it declared: the next reader cannot tell a law from a leftover, and
 * every one of these was written as though it were a law.
 *
 * Deliberately narrow: this reads only `tokens.css`, and only its declarations. A literal that
 * should have been a token is a different defect with its own guards (`icon-scale`, `leading`,
 * `surfaces`), and a rule in `app.css` that declares a local variable for its own subtree is not
 * part of the sheet's vocabulary at all.
 */

/**
 * A SCALE IS DECLARED WHOLE.
 *
 * The one exemption, and the reason it is not a hole in the rule: a spacing scale is a single
 * decision with eight steps, so a run of 1-8 missing 7 is worse than a step nothing spends. It
 * invites the next 28px to be typed as a literal, which is the defect the scale exists to prevent -
 * and the design system's own sheet declares the same eight and spends this one.
 *
 * Nothing else belongs here. If a token wants adding to this list, the question to answer first is
 * why it is not either wired or deleted.
 */
const DECLARED_WHOLE = new Set(['--space-7']);

/** Every source that could spend a token: the app's own CSS, its components, and the catalogue. */
function sources(dir: string, found: string[] = []) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) sources(path, found);
    else if (/\.(?:svelte|css|ts|js)$/u.test(path)) found.push(path);
  }

  return found;
}

const TOKENS = new URL('../src/tokens.css', import.meta.url).pathname;

const files = [
  ...sources(new URL('../src', import.meta.url).pathname),
  ...sources(new URL('../stories', import.meta.url).pathname),
].filter((path) => path !== TOKENS);

const declared = [...tokensStylesheet.matchAll(/^\s*(?<token>--[\w-]+):/gmu)].map(
  (match) => match.groups?.token ?? '',
);

/**
 * What the app spends, from outside the sheet.
 *
 * A tier read through a template literal - `var(--icon-${size})` in `Icon.svelte` - spends the
 * whole family, so the family is credited when the interpolation is present. Written as the exact
 * prefix rather than as a general rule: a wildcard here would excuse any token whose name happens
 * to start like another's.
 */
const spent = new Set<string>();
for (const file of files) {
  const text = readFileSync(file, 'utf8');
  for (const match of text.matchAll(/var\(\s*(?<token>--[\w-]+)/gu)) {
    spent.add(match.groups?.token ?? '');
  }
  if (text.includes('--icon-${')) {
    for (const token of declared) if (token.startsWith('--icon-')) spent.add(token);
  }
}

/**
 * A token the sheet reads to build another one counts only if that one is itself reachable.
 *
 * Without the closure a dead token keeps its dependencies alive: `--rhythm-card-foot-band` was
 * derived from `--row-pad-default`, and while the band was declared the padding looked spent even
 * though the only thing spending it was also dead.
 */
function reachable() {
  const uses = new Map<string, Set<string>>();
  const blocks = tokensStylesheet.split(/^\s*(--[\w-]+):/gmu).slice(1);
  for (let index = 0; index < blocks.length; index += 2) {
    const token = blocks[index];
    const value = (blocks[index + 1] ?? '').split(';')[0];
    const refs = uses.get(token) ?? new Set<string>();
    for (const match of value.matchAll(/var\(\s*(?<token>--[\w-]+)/gu)) {
      refs.add(match.groups?.token ?? '');
    }
    uses.set(token, refs);
  }

  const live = new Set([...spent].filter((token) => declared.includes(token)));
  for (let grew = true; grew;) {
    grew = false;
    for (const token of live) {
      for (const ref of uses.get(token) ?? []) {
        if (declared.includes(ref) && !live.has(ref)) {
          live.add(ref);
          grew = true;
        }
      }
    }
  }

  return live;
}

describe('the token sheet [Unit]', () => {
  it('has tokens and sources to check', () => {
    // A sweep that stands down when its pattern stops matching is not a sweep.
    expect(declared.length).toBeGreaterThan(200);
    expect(files.length).toBeGreaterThan(100);
    expect(spent.size).toBeGreaterThan(100);
  });

  it('declares nothing it does not read', () => {
    const live = reachable();
    const orphans = declared.filter((token) => !live.has(token) && !DECLARED_WHOLE.has(token));
    expect(
      orphans,
      'wire it at the distance, state or size it names, or delete it and say why in the commit',
    ).toEqual([]);
  });

  it('exempts only what it can justify', () => {
    // The exemption is for a scale declared whole. A name that leaves the sheet has to leave this
    // list with it, or the list starts excusing tokens nobody checked.
    expect([...DECLARED_WHOLE].filter((token) => !declared.includes(token))).toEqual([]);
  });
});
