import { describe, expect, it } from 'vitest';

import { durationParts, durationSeconds, formatDuration } from '#lib/duration.js';

/**
 * The seconds are what the wire carries; the parts are what a field shows.
 *
 * The whole of this module's risk is in the crossing: a value the server reads
 * as an instruction can be produced by a box somebody merely emptied, and 0
 * seconds is the most expensive instruction there is - check every repository's
 * tree on every sweep.
 */
describe('duration [Unit]', () => {
  it('says a number of seconds in the largest unit that divides it exactly', () => {
    expect(durationParts(3600)).toEqual({ amount: 1, unit: 'hours' });
    expect(durationParts(86_400)).toEqual({ amount: 1, unit: 'days' });
    // Not "an hour and a half of an hour": nothing is rounded away.
    expect(durationParts(5400)).toEqual({ amount: 90, unit: 'minutes' });
  });

  /**
   * Zero divides by every unit, so the loop would answer with the largest one
   * offered and render "0 days" - which the field then refuses to apply,
   * because it asks for at least 1.
   */
  it('says zero in the smallest unit offered', () => {
    expect(durationParts(0, ['minutes', 'hours'])).toEqual({ amount: 0, unit: 'minutes' });
  });

  it('turns what a field shows back into seconds', () => {
    expect(durationSeconds({ amount: 2, unit: 'hours' })).toBe(7200);
    // A value the server accepts, and means "every sweep".
    expect(durationSeconds({ amount: 0, unit: 'hours' })).toBe(0);
  });

  /**
   * The case this guard exists for, and the one it used to miss.
   *
   * Svelte binds an emptied `type="number"` box to **null**, and `null * 3600`
   * is 0 rather than NaN - so a guard that checked the product caught `1e999`
   * and let an emptied box through as 0. `min` without `required` does not stop
   * a form either, so nothing else was going to catch it: clearing the box and
   * pressing Apply saved the most expensive interval there is.
   *
   * Typed as a number and asserted against what a binding really produces,
   * because the type is exactly the thing that was wrong here.
   */
  it('asks for nothing where the box is not asking for a number', () => {
    const emptied = { amount: null as unknown as number, unit: 'hours' } as const;

    expect(durationSeconds(emptied)).toBeNull();
    expect(durationSeconds({ amount: Number.NaN, unit: 'hours' })).toBeNull();
    // Typing `1e999` into the box is how this one arrives: it parses to
    // Infinity, which `JSON.stringify` writes as null - which the server reads
    // as inheriting, quietly undoing an override. Written as the named constant
    // because the literal is one `no-loss-of-precision` refuses to have in the
    // source, which is the same fact from the other side.
    expect(durationSeconds({ amount: Number.POSITIVE_INFINITY, unit: 'hours' })).toBeNull();
    expect(durationSeconds({ amount: -1, unit: 'hours' })).toBeNull();
  });

  it('says one of something in the singular', () => {
    expect(formatDuration(3600)).toBe('1 hour');
    expect(formatDuration({ amount: 3, unit: 'days' })).toBe('3 days');
  });
});
