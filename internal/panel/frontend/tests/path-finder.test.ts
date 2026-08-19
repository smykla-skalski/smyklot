// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';

import PathFinder from '../src/lib/components/PathFinder.svelte';

/**
 * The finder paints the characters somebody typed, found in the path they
 * landed in.
 *
 * Which characters those are comes from `matchPath`, as offsets into the path -
 * and offsets are the whole risk here: they are counted in code units, and the
 * obvious way to walk a string for rendering counts code points.
 */
function open(paths: { path: string; repositories: number }[]) {
  render(PathFinder, {
    props: { value: '', paths, repositories: paths.length, label: 'File path' },
  });

  const input = screen.getByRole('combobox');

  return { input, type: (text: string) => fireEvent.input(input, { target: { value: text } }) };
}

/** Every character the finder drew as matched, in the order it drew them. */
function highlighted(): string {
  return [...document.querySelectorAll('.finder-opt .is-match')]
    .map((one) => one.textContent ?? '')
    .join('');
}

describe('PathFinder [Unit]', () => {
  it('paints the characters that were typed', async () => {
    const finder = open([{ path: 'renovate.json', repositories: 3 }]);
    await finder.type('ren');

    expect(highlighted()).toBe('ren');
  });

  /**
   * A path holding an astral character, which git permits and this drew wrong.
   *
   * `positions` are code-unit offsets. Spreading a string iterates by code
   * point, so mapping the spread's index onto those offsets drifts by one for
   * every astral character before it - here every letter after the emoji was
   * painted one place to the left, and the emoji itself was painted as a match
   * it had nothing to do with. Splitting by code unit instead is not the fix
   * either: it cuts the surrogate pair in half and renders two replacement
   * characters.
   */
  it('paints the right characters in a path holding an emoji', async () => {
    const finder = open([{ path: 'a😀bc.json', repositories: 1 }]);
    await finder.type('bc');

    expect(highlighted()).toBe('bc');
  });

  it('keeps a surrogate pair whole rather than splitting it', async () => {
    const finder = open([{ path: 'a😀bc.json', repositories: 1 }]);
    await finder.type('a');

    const drawn = [...document.querySelectorAll('.finder-opt .base > span')].map(
      (one) => one.textContent ?? '',
    );

    // The emoji is one drawn character, not two halves of one.
    expect(drawn).toContain('😀');
    expect(drawn.join('')).toBe('a😀bc.json');
  });
});
