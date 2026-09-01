import type { ChipTone } from './components/Chip.svelte';
import type { IconName } from './components/Icon.svelte';

/**
 * The tones a filtered value can be drawn in.
 *
 * `default` used to be one of them, and it was the only member that did not name
 * a colour - it named the absence of one, and the menu read it as "not really
 * a tone" and drew a bare word. So every value whose column is drawn in the
 * neutral chip - Running in the queue, Cancelled beside it, Removed and Declined
 * in the access tables - lost its chip in the menu while the four values around
 * it kept theirs. Omitting `tone` is what asks for a plain word; a tone that is
 * present is always drawn.
 */
export type FilterTone =
  'neutral' | 'signal' | 'on' | 'off' | 'valid' | 'missing' | 'invalid' | 'bypassed';

/** A filter's vocabulary in the chip vocabulary, so a value looks the same here as in its column. */
export function chipToneOf(tone: FilterTone): ChipTone {
  if (tone === 'valid' || tone === 'on') return 'clear';
  if (tone === 'invalid' || tone === 'off') return 'stop';
  if (tone === 'bypassed') return 'warning';
  if (tone === 'missing') return 'absent';
  if (tone === 'signal') return 'signal';

  return 'neutral';
}

export interface FilterOption {
  value: string;
  label: string;
  description?: string;
  exclusive?: boolean;
  /**
   * Draws this value as the chip its column draws, rather than as a plain label.
   *
   * It used to put an invented coloured dot in a column of its own between the checkbox and the
   * words, which meant two small marks side by side and a reader having to decide which one was the
   * control. Drawn as the chip, the row reads control, then the thing being filtered - one mark,
   * one object - and the menu shows exactly what the column shows.
   */
  tone?: FilterTone;
  /** The glyph that chip carries, where the column's chip has one. */
  icon?: IconName;
}

export interface FilterSection {
  label?: string;
  options: readonly FilterOption[];
}

export function updateFilterSelection(
  selected: readonly string[],
  option: FilterOption,
  options: readonly FilterOption[],
  multiple: boolean,
  fallbackValue?: string,
): string[] {
  if (!multiple || option.exclusive === true) return [option.value];

  const exclusiveValues = new Set(
    options.filter((candidate) => candidate.exclusive === true).map((candidate) => candidate.value),
  );
  const next = selected.filter((value) => !exclusiveValues.has(value));
  const index = next.indexOf(option.value);
  if (index === -1) next.push(option.value);
  else next.splice(index, 1);

  return next.length === 0 && fallbackValue !== undefined ? [fallbackValue] : next;
}
