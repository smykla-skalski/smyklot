import { readFileSync, readdirSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

/**
 * Source stays text.
 *
 * `merge.ts` used a byte no JSON text can hold as its "this value is absent"
 * sentinel, and typed it - a raw NUL sitting inside a string literal. It works,
 * every check passed it, and nothing on the page was wrong. What it cost was
 * the file: `grep` answered `binary file matches` and printed no line, so a
 * sweep for a symbol skipped it silently, `git diff` showed `Binary files
 * differ`, and an editor drew nothing at all where the byte was.
 *
 * The escape says exactly the same thing to the runtime. Whether it is typed or
 * escaped is invisible in the one place a reader looks and decides whether
 * every text tool can still read the file, which is why it is a rule rather
 * than a preference.
 *
 * Tab, newline and carriage return are how text is laid out, so those three are
 * the exceptions. Anything else in this range is either a mistake or a value
 * that wanted an escape.
 */

const root = new URL('../src/', import.meta.url);

/** Extensions a person edits by hand. Fonts and images are binary on purpose. */
const TEXT = ['.ts', '.js', '.svelte', '.css', '.html', '.json', '.md', '.svg'];

/** Tab, newline and carriage return: the three that lay text out. */
const LAYOUT = new Set([9, 10, 13]);

/**
 * Where the first control character is, or -1.
 *
 * A scan rather than a regular expression, because the pattern for this is what
 * `no-control-regex` forbids - a rule against writing the very characters a
 * check for them has to name. Disabling a lint rule inside the test that
 * enforces the same idea beside it reads as an exception; a loop needs none.
 */
function offendingAt(source: string): number {
  for (let at = 0; at < source.length; at++) {
    const code = source.charCodeAt(at);
    if (LAYOUT.has(code)) continue;
    if (code < 32 || code === 127) return at;
  }

  return -1;
}

function textSources(): string[] {
  return readdirSync(root, { recursive: true, encoding: 'utf8' }).filter((file) =>
    TEXT.some((extension) => file.endsWith(extension)),
  );
}

const sources = textSources();

describe('source a text tool can read', () => {
  it('has sources to check', () => {
    expect(sources.length).toBeGreaterThan(100);
  });

  it.each(sources)('writes no control character in %s', (file) => {
    const source = readFileSync(new URL(file, root), 'utf8');
    const at = offendingAt(source);

    expect(
      at,
      at < 0
        ? ''
        : `${file} holds U+${(source.codePointAt(at) ?? 0)
            .toString(16)
            .padStart(4, '0')
            .toUpperCase()} at offset ${at}; write it as an escape, ` +
            'or every text tool reads this file as binary',
    ).toBe(-1);
  });
});
