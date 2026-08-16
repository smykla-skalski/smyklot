import { describe, expect, it } from 'vitest';

import { componentSources, markupOf } from './support/markup';

/**
 * One component floats a layer, and the rest ask it to.
 *
 * There were six, and they disagreed. Three rendered into the page, where the
 * first ancestor that scrolls clips them; three re-implemented dismissal, and the
 * same toggle bug was found and fixed twice because each had its own copy of it.
 * The cost of that was never one big failure - it was that fixing a layer bug
 * fixed it in one place out of six.
 *
 * So these hold the shape rather than the pixels: what may open a layer, what may
 * position one, and that the components which have one still get it from the
 * primitive. Checked as source, because the runtime here has no DOM and no top
 * layer to put anything in.
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
  const users = [
    'ActionMenu.svelte',
    'FilterMenu.svelte',
    'HistoryDisplayMenu.svelte',
    'IdentityBar.svelte',
    'LoginField.svelte',
    'RolePicker.svelte',
  ];

  it.each(users)('%s gets its layer from the primitive', (file) => {
    expect(read(file)).toMatch(/<Popover[\s\n]/u);
  });

  it('keeps the combobox out of the platform light dismiss', () => {
    // An auto popover dismisses on any pointerdown, including one in the field
    // being typed into, which is the whole interaction here.
    expect(read('LoginField.svelte')).toMatch(/dismiss="manual"/u);
  });
});
