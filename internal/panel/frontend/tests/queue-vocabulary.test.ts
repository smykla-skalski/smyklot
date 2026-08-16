import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { queueState } from '../src/lib/queue';
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
    const invented = [...new Set(seeded.filter((state) => !declared.includes(state)))];

    expect(invented, `seeded but never emitted:\n  ${invented.join('\n  ')}`).toEqual([]);
  });
});
