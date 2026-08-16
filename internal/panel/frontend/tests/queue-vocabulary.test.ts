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

/** The states `queueState` names, read from its own switch rather than restated here. */
function handledStates(): string[] {
  const source = readFileSync(new URL('../src/lib/queue.ts', import.meta.url), 'utf8');
  const start = source.indexOf('export function queueState');
  const body = source.slice(start, source.indexOf('\n}', start));

  return [...body.matchAll(/case '(?<state>[a-z_]+)':/gu)].map(
    (match) => match.groups?.state ?? '',
  );
}

function requestWith(state: string): PendingCIRequest {
  return { last_observed_state: state } as PendingCIRequest;
}

describe('the queue vocabulary [Unit]', () => {
  const declared = declaredStates();

  it('reads the states the service declares', () => {
    expect(declared).toEqual(
      expect.arrayContaining(['passing', 'pending', 'failing', 'no_checks', 'indeterminate']),
    );
  });

  /* The default arm says "Awaiting first check", which is the honest answer for a request with no
     observation yet - and the wrong one for every state that has a name. */
  it.each(declaredStates())('draws %s as a state of its own', (state) => {
    expect(queueState(requestWith(state)).label).not.toBe('Awaiting first check');
  });

  it('invents no state the service cannot emit', () => {
    const invented = handledStates().filter((state) => !declared.includes(state));

    expect(invented, `handled here but never emitted:\n  ${invented.join('\n  ')}`).toEqual([]);
  });
});
