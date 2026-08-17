/**
 * A list somebody types one entry per line.
 *
 * Three forms on the sync page take one - the rulesets to leave alone, the
 * paths to remove, the files a repository keeps out - and all three read and
 * write it the same way. Written out in each of them, the third copy was where
 * a fourth would have quietly dropped the trimming.
 */

/**
 * What a list looks like in a box.
 *
 * A list nobody has set reads as an empty box rather than as a crash, because
 * the documents these come out of leave a list they have nothing for out
 * entirely.
 */
export function lines(values: readonly string[] | undefined): string {
  return (values ?? []).join('\n');
}

/**
 * What a box says, as a list.
 *
 * Trimmed and emptied, because a line somebody left blank or indented is not an
 * entry: an exclusion of " LICENSE" matches nothing, and matching nothing is
 * indistinguishable from working until the day it matters.
 */
export function asList(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '');
}
