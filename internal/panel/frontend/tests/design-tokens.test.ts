import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

/**
 * Every token a stylesheet reads is a token something declares.
 *
 * A custom property nothing declares does not fall back to anything sensible. It
 * is invalid at computed-value time, which for an inherited property means the
 * parent's value and for everything else the initial one - so the rule does not
 * fail, it quietly resolves to whatever was already there. `color:
 * var(--surface)` on a dark tooltip inherited the dark ground it stood on and
 * painted the sentence in it; `color: var(--text-strong)` on the sync page did
 * the same thing less visibly. Both shipped, and neither was a typo anybody could
 * see by reading the file: the name looks exactly like a token, and half the
 * panel's real tokens are spelled the same way.
 *
 * So this reads the names rather than the colours. It is the cheap half of the
 * check that `palette-aliases.test.ts` does properly for the ones that do exist.
 */
const SOURCE = new URL('../src/', import.meta.url);

/** `var(--x)` with nothing after it: a read with no fallback to save it. */
const BARE_READ = /var\(\s*(--[\w-]+)\s*\)/gu;
const DECLARED = /(--[\w-]+)\s*:/gu;
/**
 * Set from the markup or from script rather than declared in a stylesheet, which
 * is how a swatch colour, a measured offset or an animation phase arrives.
 */
const BOUND = /(?:style:(--[\w-]+)|setProperty\(\s*['"`](--[\w-]+))/gu;

/** Bits UI publishes these onto the floating element it positions. */
const PROVIDED = /^--bits-/u;

/**
 * Comments, gone before anything is counted. Every rule here is written down
 * beside the code it governs, so the files are full of prose that names tokens -
 * including the two that this check was written for, which would otherwise be
 * reported by the very sentences explaining them.
 */
const COMMENTS = [/\/\*[\s\S]*?\*\//gu, /<!--[\s\S]*?-->/gu, /(?<!:)\/\/[^\n]*/gu];

function code(source: string): string {
  return COMMENTS.reduce((text, pattern) => text.replaceAll(pattern, ' '), source);
}

function sources(directory: URL): string[] {
  const found: string[] = [];
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const child = new URL(`${entry.name}${entry.isDirectory() ? '/' : ''}`, directory);
    if (entry.isDirectory()) found.push(...sources(child));
    else if (/\.(?:svelte|css)$/u.test(entry.name)) found.push(child.pathname);
  }

  return found;
}

interface Read {
  token: string;
  file: string;
}

function scan(): { declared: Set<string>; bound: Set<string>; reads: Read[] } {
  const declared = new Set<string>();
  const bound = new Set<string>();
  const reads: Read[] = [];

  for (const file of sources(SOURCE)) {
    const source = code(readFileSync(file, 'utf8'));
    const name = file.slice(file.indexOf(join('src', '')));
    for (const match of source.matchAll(DECLARED)) declared.add(match[1] as string);
    for (const match of source.matchAll(BOUND)) bound.add((match[1] ?? match[2]) as string);
    for (const match of source.matchAll(BARE_READ)) {
      reads.push({ token: match[1] as string, file: name });
    }
  }

  return { declared, bound, reads };
}

describe('the design tokens [Unit]', () => {
  const { declared, bound, reads } = scan();

  it('are read from the stylesheets, and there are some', () => {
    // The precondition. A pattern that stops matching leaves the check below
    // passing over an empty list, which is the one failure it could not report.
    expect(declared.size).toBeGreaterThan(200);
    expect(reads.length).toBeGreaterThan(500);
    expect(declared).toContain('--text-primary');
  });

  it('are all declared somewhere, or set where they are used', () => {
    const undeclared = reads.filter(
      ({ token }) => !declared.has(token) && !bound.has(token) && !PROVIDED.test(token),
    );

    expect(
      undeclared,
      `these are read with no fallback and nothing declares them:\n${undeclared
        .map(({ token, file }) => `  ${token} in ${file}`)
        .join('\n')}`,
    ).toEqual([]);
  });
});
