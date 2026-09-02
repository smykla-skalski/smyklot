/**
 * A component's source with its commentary taken out.
 *
 * Several of these checks work by looking for something a component must not
 * contain, and the comments are where the thing it must not contain gets
 * explained. `Modal` describes what happens to a dialog written inside a closed
 * `<details>`; a sweep for disclosure menus found that sentence and reported it.
 * A rule whose own explanation breaks it is a rule nobody will keep.
 */

import { readFileSync, readdirSync } from 'node:fs';

const components = new URL('../../src/lib/components/', import.meta.url);

/**
 * Every component, as `[filename, source]`.
 *
 * Several of these checks are sweeps over the whole component directory, and
 * each had grown its own copy of this three-line read - which is how one of them
 * ends up scanning a set the others do not.
 */
export function componentSources(): (readonly [string, string])[] {
  return readdirSync(components)
    .filter((file) => file.endsWith('.svelte'))
    .map((file) => [file, readFileSync(new URL(file, components), 'utf8')] as const);
}

/**
 * Strips a pattern until the text stops changing.
 *
 * One pass is not enough for anything that can nest or overlap: removing the
 * comment in `<!--<!-- -->` leaves a bare `<!--` behind, and a single pass would
 * hand that back as if it were markup. Repeating until nothing changes is what
 * makes the result actually free of them.
 *
 * Four suites arrived at this idea separately and two of them wrote the single
 * pass, which is why it is exported rather than kept here: CodeQL reads a lone
 * `replace(/<!--[\s\S]*?-->/g, '')` as an incomplete sanitizer and is right to -
 * the shape is the same whether the text came off disk or off the wire, and a
 * helper nobody can find is a helper everybody rewrites.
 *
 * The replacement is a parameter because a token sweep needs a SPACE: joining
 * the words either side of a removed comment invents an identifier that was
 * never written.
 */
export function stripAll(source: string, pattern: RegExp, replacement = ''): string {
  let current = source;
  let previous: string;
  do {
    previous = current;
    current = current.replaceAll(pattern, replacement);
  } while (current !== previous);

  return current;
}

/** The three ways this codebase writes a comment, in the order they nest. */
export const HTML_COMMENT = /<!--[\s\S]*?-->/gu;
export const BLOCK_COMMENT = /\/\*[\s\S]*?\*\//gu;
export const LINE_COMMENT = /^\s*\/\/.*$/gmu;

/** Source with HTML comments, block comments and line comments removed. */
export function markupOf(source: string): string {
  const withoutHTML = stripAll(source, HTML_COMMENT);
  const withoutBlocks = stripAll(withoutHTML, BLOCK_COMMENT);

  return stripAll(withoutBlocks, LINE_COMMENT);
}
