import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { queueState, shortAge, sinceLabel } from '../src/lib/queue';
import type { PendingCIRequest } from '../src/lib/types';

/**
 * The queue spells CI states the way the service spells them.
 *
 * `last_observed_state` crosses the wire as a bare string - `types.ts` does not narrow it to a
 * union, and `root_dto.go` passes the column through untouched - so nothing between the reconciler
 * and the chip checks that the two ends agree. They did not: the panel had a case for `running` and
 * one for `unreadable`, which the service has never emitted, and no case for `indeterminate`, which
 * it does. A request the service could not read the checks for drew the same dashed chip as one it
 * had never looked at, and the Checks filter offered two values that could not match a row.
 *
 * The Go declaration is the vocabulary of record, so this reads it rather than restating it. A
 * state added there fails here until the panel gives it a chip.
 */

const GO_SOURCE = new URL('../../../pendingci/types.go', import.meta.url);

/** The `ObservedState` constants, read from the const block that declares them. */
function declaredStates(): string[] {
  const source = readFileSync(GO_SOURCE, 'utf8');
  const block = source.slice(
    source.indexOf('ObservedState = "'),
    source.indexOf(')', source.indexOf('ObservedState = "')),
  );
  const states = [...block.matchAll(/ObservedState = "(?<state>[a-z_]+)"/gu)].map(
    (match) => match.groups?.state ?? '',
  );
  if (states.length < 3) throw new Error(`ObservedState parsed to only ${states.length} values`);

  return states;
}

/**
 * The states `queueState` names, read from its own switch rather than restated here.
 *
 * Guarded like `declaredStates`, and for a sharper reason: this one answers a question of the form
 * "is anything here wrong", so a parse that reads nothing is indistinguishable from a clean result.
 * Rewriting the switch as a lookup table would silently retire the check - including a rewrite that
 * put `running` and `unreadable` back.
 */
function handledStates(): string[] {
  const source = readFileSync(new URL('../src/lib/queue.ts', import.meta.url), 'utf8');
  const start = source.indexOf('export function queueState');
  const body = source.slice(start, source.indexOf('\n}', start));
  const states = [...body.matchAll(/case '(?<state>[a-z_]+)':/gu)].map(
    (match) => match.groups?.state ?? '',
  );
  if (states.length < 4) {
    throw new Error(
      `queueState parsed to only ${states.length} cases - if it no longer switches on the state, ` +
        'this parse needs rewriting rather than deleting',
    );
  }

  return states;
}

function requestWith(state: string): PendingCIRequest {
  return { last_observed_state: state } as PendingCIRequest;
}

/**
 * What an unnamed state draws, taken from the code rather than copied out of it.
 *
 * Written as a literal, the assertion below rested on a label in the file it was checking: renaming
 * the default arm's copy disarmed it, and nothing failed.
 */
const UNNAMED = queueState(requestWith('a state no service will ever emit')).label;

describe('the queue vocabulary [Unit]', () => {
  const declared = declaredStates();

  it('reads the states the service declares', () => {
    expect(declared).toEqual(
      expect.arrayContaining(['passing', 'pending', 'failing', 'no_checks', 'indeterminate']),
    );
  });

  /* The default arm is the honest answer for a request with no observation yet, and the wrong one
     for every state that has a name. */
  it.each(declaredStates())('draws %s as a state of its own', (state) => {
    expect(queueState(requestWith(state)).label).not.toBe(UNNAMED);
  });

  /* The note on `queueState` says a state is a tone AND a distinct shape AND a word, because three
     of the pairs in this column collapse under one dichromacy or another. It was not true: a
     request with no observation yet and one whose repository has no CI at all were both a grey
     dashed circle, so two of the six were one drawing with two captions. No two may share a glyph,
     and a shared tone is only allowed where the glyph differs. */
  it('draws no two states of the Checks column the same way', () => {
    const drawn = [...declared, ''].map((state) => queueState(requestWith(state)));
    const glyphs = drawn.map((one) => one.icon);
    const both = drawn.map((one) => `${one.tone} ${one.icon}`);

    expect(new Set(glyphs).size, `glyphs: ${glyphs.join(', ')}`).toBe(drawn.length);
    expect(new Set(both).size, `tone and glyph: ${both.join(', ')}`).toBe(drawn.length);
  });

  it('invents no state the service cannot emit', () => {
    const invented = handledStates().filter((state) => !declared.includes(state));

    expect(invented, `handled here but never emitted:\n  ${invented.join('\n  ')}`).toEqual([]);
  });

  /* The fixture is where the vocabulary is read in practice - it is what the panel is developed and
     measured against, so a state it seeds that the service cannot send is a view of the product
     nobody will ever see. One seed already spelled `running`, and was hidden only because its row
     had finished, which sends the chip down a different function. */
  it('is the vocabulary the development fixture seeds', () => {
    const source = readFileSync(new URL('../dev/mock-server.ts', import.meta.url), 'utf8');
    const seeded = [...source.matchAll(/last_observed_state: '(?<state>[a-z_]*)'/gu)].map(
      (match) => match.groups?.state ?? '',
    );
    if (seeded.length === 0) throw new Error('no pending-CI seeds found in the mock server');
    /* The empty string is not one of the constants and is still a state the service sends: it is
       the column's own default, and a request carries it from the command until the reconciler
       first looks. It is the sixth thing the Checks column can draw, so the fixture seeds it. */
    const invented = [
      ...new Set(seeded.filter((state) => state !== '' && !declared.includes(state))),
    ];

    expect(invented, `seeded but never emitted:\n  ${invented.join('\n  ')}`).toEqual([]);
  });

  /* The Checks column's own filter list used to be compared against the states the column
     can draw, and it lived in `QueueView` - the queue's table, which went with the page
     that was its only way in. The vocabulary above is what survives it, and it is the
     half that matters: every state the service can emit is named, named distinctly, and
     seeded. There is no second list to disagree with any more. */

  /* And the reason the line above is allowed to say that. `Arm` names its columns and
     `last_observed_state` is not among them, so the row takes the migration's `DEFAULT ''`. The day
     the insert starts setting a state, this seed is a view of the product nobody will see - and
     `queueState`'s default arm stops being reachable. */
  it('leaves the first observed state to the column default', () => {
    const arm = readFileSync(
      new URL('../../../storage/sqlstore/pending_ci.go', import.meta.url),
      'utf8',
    );
    const insert = arm.slice(
      arm.indexOf('INSERT INTO pending_ci_requests'),
      arm.indexOf('RETURNING id', arm.indexOf('INSERT INTO pending_ci_requests')),
    );
    if (insert === '') throw new Error('no pending-CI insert found');

    expect(insert).not.toContain('last_observed_state');
  });
});

/**
 * How long ago, said twice: once as a measure and once as a phrase.
 *
 * The column's form has to be a number and a unit, because a column is sized by its
 * widest value and "just now" is half again as wide as any of the rest - it alone set
 * the width of the Armed column and the Finished one. The phrase is still what a
 * sentence takes, so the two forms diverge on the first minute and nowhere else, and
 * that is what these hold: `sinceLabel` is written on top of `shortAge` and used to be
 * written on top of the words it returned.
 */
describe('the age a queue row shows [Unit]', () => {
  const now = Date.parse('2026-08-17T12:00:00Z');
  const ago = (ms: number): string => new Date(now - ms).toISOString();

  it.each([
    ['the first minute', 30_000, 'now'],
    ['minutes', 59 * 60_000, '59 min'],
    ['hours', 23 * 3_600_000, '23 hr'],
    ['days', 6 * 86_400_000, '6 d'],
    ['weeks', 99 * 7 * 86_400_000, '99 wk'],
  ])('measures %s', (_case, elapsed, expected) => {
    expect(shortAge(ago(elapsed), now)).toBe(expected);
  });

  it('says the first minute in words where there is room for them', () => {
    expect(sinceLabel(ago(30_000), now)).toBe('just now');
    expect(sinceLabel(ago(59 * 60_000), now)).toBe('59 min ago');
  });

  it('takes no preposition on a timestamp it cannot read', () => {
    expect(shortAge('not a time', now)).toBe('not a time');
    expect(sinceLabel('not a time', now)).toBe('not a time');
    // The word the first minute is measured as, arriving as the value itself.
    expect(sinceLabel('now', now)).toBe('now');
  });
});
