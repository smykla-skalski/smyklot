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
  for (const unit of offered) {
    if (seconds % UNIT_SECONDS[unit] === 0) {
      return { amount: seconds / UNIT_SECONDS[unit], unit };
    }
  }
  const smallest = offered.at(-1) ?? 'seconds';

  return { amount: seconds / UNIT_SECONDS[smallest], unit: smallest };
}

export function durationSeconds(parts: DurationParts): number {
  return Math.round(parts.amount * UNIT_SECONDS[parts.unit]);
}

/** "1 hour", "3 days" - singular where the number is one. */
export function formatDuration(value: DurationParts | number): string {
  const parts = typeof value === 'number' ? durationParts(value) : value;
  const word = parts.amount === 1 ? parts.unit.slice(0, -1) : parts.unit;

  return `${parts.amount} ${word}`;
}
