import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { componentSources, markupOf } from './support/markup';

/**
 * NO OPTIONS OBJECT MIXES THE TWO FAMILIES.
 *
 * `Intl.DateTimeFormat` and `toLocaleString` take a date-time format two ways, and
 * they cannot be crossed. `dateStyle`/`timeStyle` name a whole shape; `weekday`,
 * `year`, `month`, `day`, `hour`, `minute`, `second`, `dayPeriod`, `era`,
 * `fractionalSecondDigits` and `timeZoneName` name the parts. ECMA-402 throws a
 * `TypeError` when an object carries one of each - before it formats anything, on
 * every engine, for every value.
 *
 * `timeZone` is NOT in that list and is the trap: it is legal beside a style, so
 * `{ dateStyle, timeStyle, timeZone }` works, and adding `timeZoneName` to say
 * WHICH zone is what throws. That is the shape the sync plan shipped, and the
 * whole page went down with `Invalid option : option` for every reader whose
 * profile carried a zone - so the crash needed a signed-in reader with a zone
 * set, which no fixture had.
 *
 * A unit test on a formatter only reaches the formatters something exports. This
 * reads the source, so an options object written inline inside a component -
 * which is where the broken one lived - is caught by the same rule.
 */

/** The component options, which a style may not be combined with. */
const COMPONENTS = [
  'weekday',
  'era',
  'year',
  'month',
  'day',
  'dayPeriod',
  'hour',
  'minute',
  'second',
  'fractionalSecondDigits',
  'timeZoneName',
];

const STYLES = ['dateStyle', 'timeStyle'];

/** Every `{ … }` handed to a date formatter, with the file it is in. */
function optionsIn(name: string, source: string): { file: string; text: string }[] {
  const code = markupOf(source);
  const found: { file: string; text: string }[] = [];

  /* The call, then the balanced object after its first comma - a regex cannot
     match nesting, and these objects hold `...(cond ? {} : { timeZone })`. */
  for (const call of code.matchAll(/(?:new Intl\.DateTimeFormat|toLocaleString)\s*\(/gu)) {
    const opened = code.indexOf('{', call.index);
    if (opened === -1) continue;
    let depth = 0;
    for (let at = opened; at < code.length; at += 1) {
      if (code[at] === '{') depth += 1;
      else if (code[at] === '}') {
        depth -= 1;
        if (depth === 0) {
          found.push({ file: name, text: code.slice(opened, at + 1) });
          break;
        }
      }
    }
  }

  return found;
}

/** Every component, plus the two shared modules that format a date for them. */
const sources: (readonly [string, string])[] = [
  ...componentSources(),
  ...(['format.ts', 'queue-words.ts'] as const).map(
    (file) => [file, readFileSync(new URL(`../src/lib/${file}`, import.meta.url), 'utf8')] as const,
  ),
];

describe('every date the panel formats [Unit]', () => {
  it('never combines a style with a component', () => {
    const crossed = sources
      .flatMap(([name, source]) => optionsIn(name, source))
      .filter((options) => {
        const styled = STYLES.some((one) => new RegExp(`\\b${one}\\s*:`, 'u').test(options.text));
        if (!styled) return false;

        return COMPONENTS.some((one) => new RegExp(`\\b${one}\\s*:`, 'u').test(options.text));
      })
      .map((options) => `${options.file}: ${options.text.replaceAll(/\s+/gu, ' ')}`);

    expect(
      crossed,
      'ECMA-402 throws a TypeError on these before formatting - name the parts, or drop the part',
    ).toEqual([]);
  });

  it('agrees with the engine about what it is refusing', () => {
    // The rule above is only worth its lines if this is what actually happens.
    expect(() =>
      new Intl.DateTimeFormat(undefined, {
        dateStyle: 'medium',
        timeStyle: 'short',
        timeZone: 'UTC',
        timeZoneName: 'short',
      } as Intl.DateTimeFormatOptions).format(new Date()),
    ).toThrow(TypeError);

    // And `timeZone` alone beside a style is fine, which is why the pair is a trap.
    expect(() =>
      new Intl.DateTimeFormat(undefined, {
        dateStyle: 'medium',
        timeStyle: 'short',
        timeZone: 'UTC',
      }).format(new Date()),
    ).not.toThrow();
  });
});
