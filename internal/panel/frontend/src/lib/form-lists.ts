/**
 * The lists the sync forms edit.
 *
 * Three of them on that page take a list somebody types one entry per line -
 * the rulesets to leave alone, the paths to remove, the files a repository
 * keeps out - and three keep a list of rows somebody adds to. Written out in
 * each form, the third copy was where a fourth would have quietly dropped the
 * trimming.
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

/**
 * Stable handles for the rows of one list, and the ids of the controls in them.
 *
 * Keyed by where a row sits rather than by what it says, because what it says
 * is what somebody is typing: a key that moved with the text would remount the
 * box on every keystroke and take the cursor with it.
 */
export function rowKeys(prefix: string): (index: number) => string {
  return (index) => `${prefix}-${index}`;
}
