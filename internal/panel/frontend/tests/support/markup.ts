/**
 * A component's source with its commentary taken out.
 *
 * Several of these checks work by looking for something a component must not
 * contain, and the comments are where the thing it must not contain gets
 * explained. `Modal` describes what happens to a dialog written inside a closed
 * `<details>`; a sweep for disclosure menus found that sentence and reported it.
 * A rule whose own explanation breaks it is a rule nobody will keep.
 */

/**
 * Strips a pattern until the text stops changing.
 *
 * One pass is not enough for anything that can nest or overlap: removing the
 * comment in `<!--<!-- -->` leaves a bare `<!--` behind, and a single pass would
 * hand that back as if it were markup. Repeating until nothing changes is what
 * makes the result actually free of them.
 */
export function stripAll(source: string, pattern: RegExp): string {
  let current = source;
  let previous: string;
  do {
    previous = current;
    current = current.replaceAll(pattern, '');
  } while (current !== previous);

  return current;
}

/** Source with HTML comments, block comments and line comments removed. */
export function markupOf(source: string): string {
  const withoutHTML = stripAll(source, /<!--[\s\S]*?-->/gu);
  const withoutBlocks = stripAll(withoutHTML, /\/\*[\s\S]*?\*\//gu);

  return stripAll(withoutBlocks, /^\s*\/\/.*$/gmu);
}
