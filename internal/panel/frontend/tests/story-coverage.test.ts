import { readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * Every component has a story, and the ones that do not are named here.
 *
 * A catalogue stops being complete not because somebody decides to leave a component
 * out, but because the next component arrives without one. This is the check that
 * says so, in the same shape as the dozen other directory sweeps beside it.
 *
 * `PENDING` is a ratchet rather than an excuse: a name may only ever leave it. A
 * component on the list that HAS gained a story fails, so the list cannot go stale,
 * and a component that is neither storied nor listed fails, so a new one cannot slip
 * in uncovered. Everything on it needs the same thing first - a stub `PanelApi` and a
 * seeded query cache, which is what `dev/fixtures.ts` is for.
 *
 * Stories live in `stories/`, deliberately outside `src/`: eleven of those other
 * sweeps read `src/lib/components` by directory listing and assert rules written for
 * app source, and a story is not app source.
 */
const components = new URL('../src/lib/components/', import.meta.url);
const stories = new URL('../stories/', import.meta.url);

/** Components still owed a story. Only ever gets shorter. */
/*
 * Empty, and that is the state to hold.
 *
 * A name may only ever be ADDED here with a reason, and the reasons that were here are
 * gone: `session.api` refusing every call (`PanelShell` backs it with `dev/fixtures.ts`
 * now), and a session with no selected workspace, which made every repository row throw
 * `Missing parameter 'account'` while resolving its own link.
 *
 * Two rules for whoever adds the next component. Seed a query cache only where the data
 * arrives through `api`; if the view takes a `fetchPage`-shaped prop, hand it a function
 * and the query resolves against that. And OPEN THE STORY IN A BROWSER - `svelte-check`
 * and `eslint` both pass on a story that renders a blank frame.
 */
const PENDING: readonly string[] = [];

function storyNames(): Set<string> {
  const found = new Set<string>();
  for (const entry of readdirSync(stories, { recursive: true, withFileTypes: true })) {
    if (entry.isFile() && entry.name.endsWith('.stories.svelte')) {
      found.add(entry.name.replace('.stories.svelte', ''));
    }
  }
  return found;
}

const componentNames = readdirSync(components)
  .filter((file) => file.endsWith('.svelte'))
  .map((file) => file.replace('.svelte', ''))
  .sort();

describe('the component catalogue [Unit]', () => {
  it('has components to story', () => {
    // A guard that stands down when it cannot run is not a guard.
    expect(componentNames.length).toBeGreaterThan(20);
  });

  const covered = componentNames.filter((name) => !PENDING.includes(name));

  it.each(covered)('has a story for %s', (name) => {
    expect(storyNames().has(name), `${name}.svelte has no story under stories/`).toBe(true);
  });

  it('names every component it has not covered yet', () => {
    const uncovered = componentNames.filter((name) => !storyNames().has(name));
    expect(uncovered.sort()).toEqual([...PENDING].sort());
  });

  it('lists nothing that no longer exists', () => {
    const gone = PENDING.filter((name) => !componentNames.includes(name));
    expect(gone, 'remove these from PENDING, they are not components any more').toEqual([]);
  });
});
