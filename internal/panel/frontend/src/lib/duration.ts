import type { DurationUnit } from '#lib/components/DurationField.svelte';

/**
 * A number of seconds, said the way somebody would say it.
 *
 * Three settings pages now hold a duration a person types - the process's file
 * list refresh interval, an installation's, and one repository's - and each was
 * about to work out "3600 is an hour" for itself. The seconds are what the wire
 * carries; these are what a field shows.
 */
export interface DurationParts {
  amount: number;
  unit: DurationUnit;
}

const UNIT_SECONDS: Record<DurationUnit, number> = {
  seconds: 1,
  minutes: 60,
  hours: 3_600,
  days: 86_400,
};

/**
 * The largest unit that divides these seconds exactly, out of the ones offered.
 *
 * Exactly, so nothing is rounded away: 90 minutes stays 90 minutes rather than
 * becoming an hour and a half of an hour. The units are given largest-first
 * here whatever order a caller offers them in.
 */
export function durationParts(
  seconds: number,
  units: readonly DurationUnit[] = ['seconds', 'minutes', 'hours', 'days'],
): DurationParts {
  const offered = [...units].sort((left, right) => UNIT_SECONDS[right] - UNIT_SECONDS[left]);
  // Zero divides by every unit, so the loop below would answer with the largest
  // one offered and render "0 days" - and then refuse to be applied, because
  // the field asks for at least 1. Zero is a value the server accepts, so it is
  // said in the smallest unit the caller offers.
  if (seconds === 0) return { amount: 0, unit: offered.at(-1) ?? 'seconds' };
  for (const unit of offered) {
    if (seconds % UNIT_SECONDS[unit] === 0) {
      return { amount: seconds / UNIT_SECONDS[unit], unit };
    }
  }
  const smallest = offered.at(-1) ?? 'seconds';

  return { amount: seconds / UNIT_SECONDS[smallest], unit: smallest };
}

/**
 * The seconds a field is asking for, or null where it is not asking for any.
 *
 * Null rather than a number, because both ways of getting one are silent
 * otherwise. Svelte binds an emptied `type="number"` box to `null`, and `min`
 * without `required` does not stop the form submitting - so `null * 3600` was
 * **0**, which the server reads as "check every sweep", the most expensive
 * answer there is, saved by clearing a box. And `1e999` is a value that field
 * accepts: it multiplies to `Infinity`, which `JSON.stringify` writes as
 * `null`, quietly turning an override back into inheriting.
 */
export function durationSeconds(parts: DurationParts): number | null {
  /* The emptied box is `null`, and `null * 3600` is 0 rather than NaN - so
     checking the product caught `1e999` and let the case this was written for
     straight through. The amount is checked before it is multiplied by
     anything. `typeof` rather than a null test, because the type says `number`
     and the value comes from a binding that does not have to agree. */
  if (typeof parts.amount !== 'number' || !Number.isFinite(parts.amount) || parts.amount < 0) {
    return null;
  }

  const seconds = Math.round(parts.amount * UNIT_SECONDS[parts.unit]);

  return Number.isFinite(seconds) && seconds >= 0 ? seconds : null;
}

/** "1 hour", "3 days" - singular where the number is one. */
export function formatDuration(value: DurationParts | number): string {
  const parts = typeof value === 'number' ? durationParts(value) : value;
  const word = parts.amount === 1 ? parts.unit.slice(0, -1) : parts.unit;

  return `${parts.amount} ${word}`;
}
