/**
 * What a repository ends up with, and what its adjustment has to say to get it.
 *
 * The panel edits the RESULT, never the patch. A person looking at a shared
 * file wants to see the file their repository will hold; asking them to write
 * a JSON merge patch is asking them to compose the answer in their head and
 * type the difference. So the surface is the composed file, and the stored
 * adjustment is derived back out of what they typed.
 *
 * This is RFC 7396 - the same rule the service composes with, spelled the same
 * way on both sides - and it has one thing it cannot express: setting a key to
 * `null`, because a `null` in a patch means "remove this key". `derivePatch`
 * says so by refusing rather than by writing a patch that means the other
 * thing.
 */

export type JsonValue =
  null | boolean | number | string | JsonValue[] | { [key: string]: JsonValue };

/** A plain object, which is the only thing RFC 7396 merges into rather than replaces. */
function isObject(value: JsonValue | undefined): value is { [key: string]: JsonValue } {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/**
 * The file one repository ends up with: the template, with its adjustment
 * merged in key by key, all the way down.
 */
export function mergePatch(target: JsonValue, patch: JsonValue): JsonValue {
  if (!isObject(patch)) return patch;

  const result: { [key: string]: JsonValue } = isObject(target) ? { ...target } : {};
  for (const [key, value] of Object.entries(patch)) {
    if (value === null) {
      delete result[key];
      continue;
    }
    result[key] = mergePatch(result[key] ?? null, value);
  }

  return result;
}

/**
 * The smallest adjustment that turns `before` into `after`.
 *
 * `undefined` where they are already the same, so a caller can tell "nothing to
 * store" from "store an empty object". A key set to `null` in `after` returns
 * `null` from the whole function instead: RFC 7396 reads that as a removal, and
 * writing a patch that means something other than what somebody typed is worse
 * than saying it cannot be stored.
 */
export function derivePatch(
  before: JsonValue,
  after: JsonValue,
): JsonValue | undefined | 'unsayable' {
  if (!isObject(before) || !isObject(after)) {
    if (same(before, after)) return undefined;

    return after;
  }

  const patch: { [key: string]: JsonValue } = {};
  for (const [key, value] of Object.entries(after)) {
    // The one value RFC 7396 cannot ask for: a `null` in a patch removes the
    // key rather than setting it to null. Already null is nothing to say.
    if (value === null) {
      if (key in before && before[key] === null) continue;

      return 'unsayable';
    }
    if (!(key in before)) {
      patch[key] = value;
      continue;
    }
    const inner = derivePatch(before[key] ?? null, value);
    if (inner === 'unsayable') return 'unsayable';
    if (inner !== undefined) patch[key] = inner;
  }
  for (const key of Object.keys(before)) {
    if (!(key in after)) patch[key] = null;
  }

  return Object.keys(patch).length === 0 ? undefined : patch;
}

function same(left: JsonValue, right: JsonValue): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

/**
 * Every key one adjustment touches, as the dotted path a reader would say it
 * with - `schedule`, `packageRules`, `hostRules.matchHost`.
 *
 * Only the leaves: an object the patch descends into is a route to a change
 * rather than the change, and naming both would say the same adjustment twice.
 */
export function patchedKeys(patch: JsonValue, prefix = ''): string[] {
  if (!isObject(patch)) return prefix === '' ? [] : [prefix];

  return Object.entries(patch).flatMap(([key, value]) => {
    const path = prefix === '' ? key : `${prefix}.${key}`;

    return isObject(value) && Object.keys(value).length > 0 ? patchedKeys(value, path) : [path];
  });
}

/**
 * The lists the template and the adjustment both set.
 *
 * The one question a merge cannot answer on its own: two lists can be joined in
 * three ways and none of them is more correct than the others, so it is asked
 * where it arises rather than guessed. Returned as JSONPath, which is how the
 * stored array rules spell a path.
 */
export function sharedArrays(template: JsonValue, patch: JsonValue, prefix = '$'): string[] {
  if (!isObject(template) || !isObject(patch)) return [];

  return Object.entries(patch).flatMap(([key, value]) => {
    const path = `${prefix}.${key}`;
    const theirs = template[key];
    if (Array.isArray(value) && Array.isArray(theirs)) return [path];

    return sharedArrays(theirs ?? null, value, path);
  });
}

/**
 * Which of the composed file's lines this repository decides.
 *
 * Read from the adjustment rather than by comparing the two files line by line:
 * the same value written out twice can occupy a different number of lines - one
 * list on one line in the template and over three here - and a text comparison
 * marks all three as the repository's, which says it changed something it left
 * alone.
 *
 * A key the adjustment names marks its whole value, closing bracket included,
 * because that is what somebody would take back by clearing it.
 */
export function markedLines(text: string, patch: JsonValue): number[] {
  if (!isObject(patch)) return [];

  const wanted = new Set(Object.keys(patch));
  const marks: number[] = [];
  let depth = 0;
  // The depth the marked value started at, or 0 while nothing is being marked.
  let marking = 0;

  text.split('\n').forEach((line, index) => {
    // Braces inside a string are text, not structure. `{` in a commit template
    // would otherwise open a level that never closes.
    const bare = line.replace(/"(?:[^"\\]|\\.)*"/g, '""');
    const opens = (bare.match(/[{[]/g) ?? []).length;
    const closes = (bare.match(/[}\]]/g) ?? []).length;
    const after = depth + opens - closes;

    if (marking > 0) {
      marks.push(index + 1);
      if (after <= marking) marking = 0;
    } else {
      const key = /^\s*"((?:[^"\\]|\\.)*)"\s*:/.exec(line);
      if (depth === 1 && key !== null && wanted.has(key[1] ?? '')) {
        marks.push(index + 1);
        if (after > depth) marking = depth;
      }
    }

    depth = after;
  });

  return marks;
}

/** Whether a path is a file this can compose - which is JSON and nothing else yet. */
export function composable(path: string): boolean {
  return path.toLowerCase().endsWith('.json');
}

/** The document, or nothing where it is not the JSON this composes. */
export function parseJson(text: string): JsonValue | undefined {
  try {
    return JSON.parse(text) as JsonValue;
  } catch {
    return undefined;
  }
}

/** Written back the way a person would type it, so a diff of two of these reads. */
export function formatJson(value: JsonValue): string {
  return `${JSON.stringify(value, null, 2)}\n`;
}
