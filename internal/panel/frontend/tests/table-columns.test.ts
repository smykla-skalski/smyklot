import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

/**
 * The repositories table's control columns.
 *
 * Every row of that table is its own grid, which makes a content-sized track
 * (`max-content`, `auto`, `min-content`) measure itself per row: the header
 * measured the word "Enablement" and the body measured a switch, and the two
 * stopped agreeing about where the column began. The columns holding controls
 * are therefore fixed lengths, and this holds them there - the failure it guards
 * against looks like a correct change and only shows up as a misaligned header.
 */
const source = readFileSync(
  fileURLToPath(new URL('../src/components/RepositoryList.svelte', import.meta.url)),
  'utf8',
);

function captured(pattern: RegExp, within: string, what: string): string {
  const value = pattern.exec(within)?.[1];
  if (value === undefined) throw new Error(`${what} was not found`);

  return value.replace(/\s+/g, ' ').trim();
}

function declaration(name: string): string {
  return captured(new RegExp(`${name}:\\s*([^;]+);`), source, name);
}

/**
 * The desktop table's own track list.
 *
 * Found by the rule it belongs to rather than by name: the component also lays
 * the same rows out as cards below the table's breakpoint, and that layout has a
 * `grid-template-columns` of its own which comes first in the file.
 */
function tableTemplate(): string {
  const rule = captured(
    /\.repositories thead tr,\s*\n\s*\.repositories tbody tr\s*\{([\s\S]*?)\}/,
    source,
    'the desktop table row rule',
  );

  return captured(/grid-template-columns:\s*([^;]+);/, rule, "the table's columns");
}

describe('repositories table columns [Unit]', () => {
  it('sizes the two control columns with fixed lengths', () => {
    const template = tableTemplate();
    expect(template).toContain('var(--enablement-column)');
    expect(template).toContain('var(--action-column)');

    for (const name of ['--enablement-column', '--action-column']) {
      // A length, not a track keyword: the keywords are the ones that resolve
      // per row and pull the header out of line with the body.
      expect(declaration(name)).toMatch(/^\d+(\.\d+)?rem$/);
    }
  });

  it('gives the text columns the approved proportions and no fixed width', () => {
    const template = tableTemplate();
    expect(template.startsWith('2fr 1fr 1.4fr')).toBe(true);
  });

  it('indents the enablement heading by the inheritance marker', () => {
    /* The label over that column has to start where the column's controls start,
       and every one of them opens with an inheritance marker. */
    expect(source).toContain(
      'padding-inline-start: calc(var(--space-3) + var(--inherit-marker-offset))',
    );
  });
});
