import { describe, expect, it } from 'vitest';

import { palettes, rootAliases, type Palette } from './theme';

/**
 * An alias follows the token it names, into every palette that changes it.
 *
 * `app.css` declares sixty-odd properties as a plain `var()` of another - `--dim: var(--text-muted)`,
 * `--ink: var(--canvas)`, the compatibility aliases and the surface shorthands alike - and each one
 * is a promise that reading either name gets the same paint.
 *
 * Custom properties break that promise in one specific way. A `var()` substitutes on the element
 * that *declares* it, so `--dim` is answered on `:root` against the panel's `--text-muted` and
 * inherits into the Root shell already resolved. The shell overrides `--text-muted` and gets a
 * `--dim` still holding the panel's. Both names are live, they disagree, and which one a component
 * happens to read decides whether it looks right - so one page draws some of its text in the
 * panel's petrol greys and the rest in the shell's violet ones. Nobody can point at a wrong
 * colour; the palette is only mostly applied, which reads as random.
 *
 * The shell already worked around this by re-declaring aliases by hand, and the hand-kept list is
 * what went stale: five had been missed, one of them landing a teal on a violet console.
 *
 * A palette overriding an alias outright is not a fault - that is a deliberate choice, stated. The
 * fault is the alias standing still while the token underneath it moves, so that is what this asks,
 * against the palette each one is derived from.
 */
/** By name rather than by position, so renaming or reordering a palette fails here and says so. */
function named(name: string): Palette {
  const found = palettes.find((palette) => palette.name === name);
  if (found === undefined) throw new Error(`there is no \`${name}\` palette`);

  return found;
}

/** Each palette and the one it is a re-skin of, which is what it must not silently keep. */
const DERIVED: readonly (readonly [Palette, Palette])[] = [
  [named('panel dark'), named('panel light')],
  [named('root light'), named('panel light')],
  [named('root dark'), named('panel dark')],
];

interface Drift {
  alias: string;
  source: string;
  kept: string;
  moved: string;
}

function drift(palette: Palette, parent: Palette): Drift[] {
  const found: Drift[] = [];
  for (const [alias, source] of rootAliases) {
    let here;
    let there;
    try {
      here = { alias: palette.color(alias), source: palette.color(source) };
      there = { alias: parent.color(alias), source: parent.color(source) };
    } catch {
      // Not a colour - a duration, a radius, a shadow. Nothing to compare.
      continue;
    }
    if (here.source !== there.source && here.alias === there.alias) {
      found.push({ alias, source, kept: here.alias, moved: here.source });
    }
  }

  return found;
}

describe('the palette aliases [Unit]', () => {
  it('are read from the stylesheet, and there are some', () => {
    // The precondition. If the block parser or the pattern ever stops matching, every check below
    // passes over an empty list and this file quietly stops being a test.
    expect(rootAliases.length).toBeGreaterThan(20);
    // A token every palette owns and every palette derives, named so the pattern
    // cannot quietly stop matching. It used to be `dim`, which was a compatibility
    // alias for this one and is gone.
    expect(rootAliases.map(([alias]) => alias)).toContain('neutral-tint');
  });

  for (const [palette, parent] of DERIVED) {
    it(`follow their token from ${parent.name} into ${palette.name}`, () => {
      const stale = drift(palette, parent);

      expect(
        stale,
        `${palette.name} moved these tokens but kept ${parent.name}'s answer for the alias:\n` +
          stale
            .map(
              (one) =>
                `  --${one.alias} stayed ${one.kept} while --${one.source} became ${one.moved}`,
            )
            .join('\n'),
      ).toEqual([]);
    });
  }
});
