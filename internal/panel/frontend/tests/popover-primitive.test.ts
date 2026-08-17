import { describe, expect, it } from 'vitest';

import { componentSources, markupOf } from './support/markup';

/**
 * Floating layers use Bits UI's purpose-built primitives.
 *
 * There were six, and they disagreed. Three rendered into the page, where the
 * first ancestor that scrolls clips them; three re-implemented dismissal, and the
 * same toggle bug was found and fixed twice because each had its own copy of it.
 * The cost of that was never one big failure - it was that fixing a layer bug
 * fixed it in one place out of six.
 *
 * These hold the architectural shape rather than pixels: no component drives a
 * platform popover directly, generic layers use the shared wrapper, and widgets
 * with stronger semantics use the matching Bits UI primitive.
 */

const PRIMITIVE = 'Popover.svelte';

const sources = componentSources();

/* Scanned without their commentary: these rules are explained in the components
   they govern, and an explanation that quotes the forbidden thing would be
   reported as the forbidden thing. */
const others = sources
  .filter(([file]) => file !== PRIMITIVE)
  .map(([file, source]) => [file, markupOf(source)] as const);

function read(file: string): string {
  return sources.find(([name]) => name === file)?.[1] ?? '';
}

describe('the popover primitive', () => {
  it('is the only component that opens a layer', () => {
    const offenders = others
      .filter(([, source]) => /\b(?:show|hide)Popover\s*\(/u.test(source))
      .map(([file]) => file);

    expect(offenders, 'call the primitive rather than driving a popover directly').toEqual([]);
  });

  it('is the only component that declares one', () => {
    const offenders = others
      .filter(([, source]) => /\spopover=/u.test(source))
      .map(([file]) => file);

    expect(offenders).toEqual([]);
  });

  it('is the only component that positions one', () => {
    // Writing left/top through the CSSOM is how a floating layer is placed here,
    // and it is the tell that a component has grown its own.
    const offenders = others
      .filter(([, source]) => /\.style\.(?:left|top)\s*=/u.test(source))
      .map(([file]) => file);

    expect(offenders).toEqual([]);
  });

  it('leaves no menu built out of a disclosure', () => {
    /*
     * `<details>` was what three of them used, and it is why they were clipped:
     * its panel is an ordinary element in the page, so a scrolling ancestor cuts
     * it off. The top layer is the whole reason the primitive exists.
     */
    const offenders = others
      .filter(([, source]) => /<details[\s>]/u.test(source))
      .map(([file]) => file);

    expect(offenders).toEqual([]);
  });
});

describe('the components that float a layer', () => {
  const genericUsers = [
    'FilterMenu.svelte',
    'HistoryDisplayMenu.svelte',
    'IdentityBar.svelte',
    'RolePicker.svelte',
  ];

  it.each(genericUsers)('%s gets its layer from the shared primitive', (file) => {
    expect(read(file)).toMatch(/<Popover[\s\n]/u);
  });

  it.each([
    ['Popover.svelte', 'Popover'],
    ['ActionMenu.svelte', 'DropdownMenu'],
    ['LoginField.svelte', 'Combobox'],
    ['AppTooltip.svelte', 'Tooltip'],
  ])('%s uses the Bits UI %s primitive', (file, primitive) => {
    const source = read(file);
    expect(source).toContain(`import { ${primitive} } from 'bits-ui'`);
    expect(source).toContain(`<${primitive}.Root`);
  });

  /**
   * A help tip is a tooltip with a question mark in front of it, so it is one:
   * wiring its own put a second tooltip appearance in the panel, and that one was
   * painted in a token nothing declares - unreadable in both themes, for as long
   * as it has existed.
   */
  it('HelpTip.svelte gets its tip from the shared tooltip', () => {
    const source = read('HelpTip.svelte');

    expect(source).toMatch(/<AppTooltip[\s\n]/u);
    expect(source).not.toContain("from 'bits-ui'");
  });
});
