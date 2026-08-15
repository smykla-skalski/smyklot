import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * A sidebar row is an icon beside a word, and both have to sit on the row's middle.
 *
 * Three different mistakes put them off it, and each one looked like a small thing on its own. The
 * label was a fixed 1.25rem box with the text centred in it, which centres a LINE box - ascender to
 * descender - and leaves the letters low by whatever the descender is worth. The row then carried a
 * compensating 1px of top padding, which is a fix that holds at one size, in one font, for one row.
 * And the icons rode `--ink-nudge`, which drops a mark to meet the ink centre of a word carrying a
 * descender: right for a mark inline with one word, wrong for a column, where it pushed three of
 * four icons down and left the fourth alone.
 *
 * What holds instead needs no arithmetic: trim the label to its glyph bounds, and let the row
 * centre both boxes. Checked as source, because the runtime here has no DOM and no cascade.
 */

const tabs = readFileSync(new URL('../src/components/ViewTabs.svelte', import.meta.url), 'utf8');

/**
 * Every character a regular expression reads as syntax.
 *
 * All of them, not the `.` these selectors happen to contain: a partial escape
 * is the kind that holds until the first caller passes something it did not
 * anticipate, and then quietly matches the wrong thing rather than failing.
 */
function quoteForPattern(literal: string): string {
  return literal.replaceAll(/[$()*+.?[\]\\^{|}]/gu, '\\$&');
}

/** The body of the first rule for a selector. */
function rule(selector: string): string {
  const pattern = new RegExp(`(?:^|\\n)\\s*${quoteForPattern(selector)}\\s*\\{([^}]*)\\}`, 'u');
  return pattern.exec(tabs)?.[1] ?? '';
}

describe('a sidebar navigation row', () => {
  it('trims the label to its glyph bounds', () => {
    // The box the row centres has to BE the letters, or centring it centres something else.
    expect(rule('.navigation-label')).toMatch(/text-box:\s*trim-both cap alphabetic/u);
  });

  it('gives the label no height of its own', () => {
    // A fixed height is a guess at what the text measures, and it is the guess that made the
    // letters sit low in the first place.
    expect(rule('.navigation-label')).not.toMatch(/(^|[^-])height:/u);
  });

  it('pads the row evenly', () => {
    // Uneven vertical padding is an optical nudge wearing a layout property.
    const padding = /padding:\s*([^;]+);/u.exec(rule('a'))?.[1]?.trim() ?? '';

    expect(padding.length).toBeGreaterThan(0);
    const parts = padding.split(/\s+/u);
    expect(parts.length, `"${padding}" should be one or two values, not four`).toBeLessThan(3);
  });

  it('does not drop the icon for words that happen to have a descender', () => {
    // This is where the descender nudge was found out: three of Settings, Repositories, Access and
    // History carry one and the fourth does not, so it moved most of the column and left the rest.
    // It has since been dropped everywhere, which the sweep below holds to.
    expect(rule('.navigation-icon')).not.toContain('--ink-nudge');
    expect(tabs).not.toContain('use:inkAlign');
  });
});

describe('the descender nudge', () => {
  it('is gone from the product', () => {
    /*
     * `--ink-nudge` dropped a mark by 0.11em whenever its label carried a descender. It was real
     * and it was measured, and it still lost: a mark that moves because the word beside it happens
     * to contain a "p" cannot line up with the identical mark one row down. Alignment between
     * neighbours beats alignment within one pair, so the whole mechanism went.
     */
    const roots = [new URL('../src/', import.meta.url)];
    const offenders: string[] = [];

    while (roots.length > 0) {
      const dir = roots.pop() as URL;
      for (const entry of readdirSync(dir, { withFileTypes: true })) {
        const child = new URL(entry.name + (entry.isDirectory() ? '/' : ''), dir);
        if (entry.isDirectory()) {
          roots.push(child);
          continue;
        }
        if (!/\.(svelte|ts|css)$/u.test(entry.name)) continue;
        const source = readFileSync(child, 'utf8');
        // The comments explaining why it is gone are allowed to name it; nothing may use it.
        if (/translate:\s*0 var\(--ink-nudge\)|use:inkAlign|--ink-nudge:\s/u.test(source)) {
          offenders.push(entry.name);
        }
      }
    }

    expect(offenders).toEqual([]);
  });
});

/**
 * How high and how low a path reaches, by walking it.
 *
 * Counting segments by eye gets this wrong - `v11` drops from wherever the tab already took the pen,
 * not from the starting point - so this follows the pen instead. Only the commands these icons use
 * are handled, and an unknown one throws rather than quietly measuring the wrong shape.
 */
function verticalExtent(d: string): { top: number; bottom: number } {
  let y = 0;
  let top = Infinity;
  let bottom = -Infinity;
  const see = (value: number): void => {
    top = Math.min(top, value);
    bottom = Math.max(bottom, value);
  };

  for (const [, command, rawArgs] of d.matchAll(/([MmLlHhVvAaZz])([^MmLlHhVvAaZz]*)/gu)) {
    const args = (rawArgs ?? '')
      .trim()
      .split(/[\s,]+|(?=-)/u)
      .filter(Boolean)
      .map(Number);
    switch (command) {
      case 'M':
      case 'L':
        y = args[1] ?? y;
        see(y);
        break;
      case 'm':
      case 'l':
        y += args[1] ?? 0;
        see(y);
        break;
      case 'V':
        y = args[0] ?? y;
        see(y);
        break;
      case 'v':
        y += args[0] ?? 0;
        see(y);
        break;
      case 'A':
        y = args[6] ?? y;
        see(y);
        break;
      case 'a':
        y += args[6] ?? 0;
        see(y);
        break;
      case 'H':
      case 'h':
      case 'Z':
      case 'z':
        break;
      default:
        throw new Error(`unhandled path command ${command}`);
    }
  }

  return { top, bottom };
}

describe('an icon in a column', () => {
  it('is drawn centred in its own box', () => {
    /*
     * The folder ran 5.5 to 20.5 in a 24-unit box, so its middle sat at 13 where the box's is 12
     * and it rode a unit low of every label beside it - which no amount of CSS centring can fix,
     * because the box was centred all along.
     *
     * Read out of the path rather than asserted as a string: what matters is where the shape ends
     * up, not how it is written.
     */
    const icons = readFileSync(new URL('../src/components/Icon.svelte', import.meta.url), 'utf8');
    const folder = /name === 'repositories'\}[\s\S]*?<path d="(?<d>M[^"]+)"/u.exec(icons)?.groups
      ?.d;

    expect(folder).toBeDefined();

    const { top, bottom } = verticalExtent(folder ?? '');

    expect(bottom).toBeGreaterThan(top);
    expect((top + bottom) / 2, 'the folder must be centred in the 24-unit box').toBeCloseTo(12, 5);
  });
});
