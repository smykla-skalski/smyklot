/**
 * What a repository ends up with, and what its adjustment has to say to get it.
 *
 * The panel edits the RESULT, never the patch. A person looking at a shared
 * file wants to see the file their repository will hold; asking them to write
 * a JSON merge patch is asking them to compose the answer in their head and
 * type the difference. So the surface is the composed file, and the stored
 * adjustment is derived back out of what they typed.
 *
 * Which means this file has one job and it is not "merge two objects": it is to
 * compose the same bytes `internal/orgsync/filemerge` composes, from the same
 * template and the same spec. It used to be RFC 7396 and nothing else, while
 * the service also honoured `strategy`, `arrays` and `deduplicate` - so a
 * repository with an append rule was SHOWN its own list replacing the
 * template's, and the file it got had both. `tests/merge-parity.test.ts` runs
 * one table of cases through this and through the Go package and fails on any
 * disagreement, because two implementations of one rule drift the moment
 * nothing compares them.
 *
 * The one thing RFC 7396 cannot express is setting a key to `null`, because a
 * `null` in a patch means "remove this key". `deriveOverrides` says so by
 * refusing rather than by storing a patch that means the other thing.
 */

/**
 * A number kept as the digits somebody typed.
 *
 * The service decodes with `UseNumber` and writes the literal back, so `1.50`
 * stays `1.50` and an identifier past 2^53 keeps its digits. A JavaScript
 * number cannot hold either, so numbers are parsed into the raw-JSON boxes the
 * platform provides and `JSON.stringify` writes them out verbatim.
 */
export interface JsonNumber {
  readonly rawJSON: string;
  /**
   * Never present at runtime, and load-bearing at compile time.
   *
   * A box is structurally `{ rawJSON: string }`, which is assignable to a
   * string-keyed record of JSON values - so narrowing a `JsonValue` to "an
   * object" left the box in the union and every index below it needed a cast.
   * An optional property of a type no JSON value has takes it out.
   */
  readonly notARecord?: undefined;
}

export type JsonValue =
  null | boolean | number | string | JsonNumber | JsonValue[] | { [key: string]: JsonValue };

/*
 * The platform's raw-JSON API, which TypeScript's libraries do not describe
 * yet. `JSON.parse`'s third reviver argument and `JSON.rawJSON` are one
 * proposal - source text in, source text out - and the panel needs both halves
 * to keep a number the way it was written.
 */
declare global {
  interface JSON {
    isRawJSON(value: unknown): value is JsonNumber;
    rawJSON(text: string): JsonNumber;
  }
}

/** A raw-JSON box, which is an object to `typeof` and a number to a reader. */
function isNumber(value: unknown): value is JsonNumber {
  return JSON.isRawJSON(value);
}

/**
 * A plain object, which is the only thing RFC 7396 merges into rather than
 * replaces.
 *
 * `unknown` in rather than `JsonValue`, so the answer is a record rather than
 * a union: a raw-JSON box is structurally a `{ rawJSON: string }`, which is
 * assignable to a string-keyed record, so narrowing a `JsonValue` leaves both
 * and every index below has to be told again which one it is.
 */
function isObject(value: unknown): value is { [key: string]: JsonValue } {
  return typeof value === 'object' && value !== null && !Array.isArray(value) && !isNumber(value);
}

/**
 * Write a key without going through its setter.
 *
 * `result[key] = value` invokes `__proto__`'s setter rather than storing a key
 * of that name: the key vanished from the composed file and, worse, the value
 * silently became the object's prototype. `JSON.parse` defines that key rather
 * than setting it, so a template holding one parsed correctly and then lost it
 * on the way through a merge.
 */
function put(target: { [key: string]: JsonValue }, key: string, value: JsonValue): void {
  Object.defineProperty(target, key, {
    configurable: true,
    enumerable: true,
    value,
    writable: true,
  });
}

/** A shallow copy that keeps a `__proto__` key a key, for the same reason. */
function copy(source: { [key: string]: JsonValue }): { [key: string]: JsonValue } {
  const result: { [key: string]: JsonValue } = {};
  for (const key of Object.keys(source)) put(result, key, source[key] as JsonValue);

  return result;
}

/**
 * The file one repository ends up with under a deep merge: the template, with
 * its adjustment merged in key by key, all the way down. RFC 7396.
 */
export function mergePatch(target: JsonValue, patch: JsonValue): JsonValue {
  if (!isObject(patch)) return patch;

  const result = isObject(target) ? copy(target) : {};
  for (const key of Object.keys(patch)) {
    const value = patch[key] as JsonValue;
    if (value === null) {
      delete result[key];
      continue;
    }
    put(result, key, mergePatch(result[key] ?? null, value));
  }

  return result;
}

/**
 * A shallow merge replaces top-level keys rather than merging into them.
 *
 * A null at the top level removes the key, as it does in a deep merge. A null
 * anywhere below one is a null value, because a shallow merge does not look
 * below the top level.
 */
function mergeShallow(target: JsonValue, patch: JsonValue): JsonValue {
  if (!isObject(patch)) return patch;

  const result = isObject(target) ? copy(target) : {};
  for (const key of Object.keys(patch)) {
    const value = patch[key] as JsonValue;
    if (value === null) {
      delete result[key];
      continue;
    }
    put(result, key, value);
  }

  return result;
}

/** How one repository composes its copy - the fields of a stored merge entry. */
export interface MergeSpec {
  /** `deep-merge` or `shallow-merge`. Empty is a deep merge. */
  strategy?: string;
  overrides?: JsonValue;
  /** In order: two rules on one document have to resolve the same way twice. */
  arrays?: readonly { path: string; strategy: string }[];
  deduplicate?: boolean;
  /** Markdown only, and only counted here - this composer reads JSON. */
  sections?: readonly unknown[];
}

/**
 * A spec that composes nothing, so the template IS what the repository holds.
 *
 * The service returns the template's own bytes for one of these rather than
 * reading and writing it back, and the difference shows: a document that goes
 * through comes out with its keys sorted. Drawing the sorted one for a
 * repository whose file will be the template verbatim is a diff nobody made.
 *
 * Three spellings arrive for an adjustment that says nothing - absent, null,
 * and the empty object - and all three mean the same thing.
 */
export function composesNothing(spec: MergeSpec): boolean {
  const overrides = spec.overrides;
  const adjusts =
    overrides !== undefined &&
    overrides !== null &&
    (!isObject(overrides) || Object.keys(overrides).length > 0);

  return (
    (spec.strategy ?? '') === '' &&
    !adjusts &&
    (spec.arrays ?? []).length === 0 &&
    spec.deduplicate !== true &&
    (spec.sections ?? []).length === 0
  );
}

/**
 * A composed file, or the reason there is not one.
 *
 * A reason rather than a fallback. Every one of these is a merge the service
 * refuses too, and drawing the template unchanged where the service will refuse
 * to write anything tells a reader the opposite of what is about to happen.
 */
export type Composed = { ok: true; value: JsonValue } | { ok: false; reason: string };

/** The file this repository ends up with, composed the way the service composes it. */
export function composeFile(template: JsonValue, spec: MergeSpec): Composed {
  const overrides = spec.overrides ?? {};
  const merged =
    spec.strategy === 'shallow-merge'
      ? mergeShallow(template, overrides)
      : mergePatch(template, overrides);

  for (const rule of spec.arrays ?? []) {
    const keys = parsePath(rule.path);
    if (typeof keys === 'string') return { ok: false, reason: keys };

    const wanted = valueAt(overrides, keys);
    if (wanted === undefined) {
      return {
        ok: false,
        reason: `No adjustment sets ${rule.path}, so there is no list to ${rule.strategy}`,
      };
    }
    if (!Array.isArray(wanted)) {
      return { ok: false, reason: `The adjustment at ${rule.path} is not a list` };
    }

    const held = valueAt(template, keys);
    const base = Array.isArray(held) ? held : [];
    const combined = mergeArrays(base, wanted, rule.strategy, spec.deduplicate === true);

    if (!setValueAt(merged, keys, combined)) {
      return { ok: false, reason: `Nothing in the composed file holds ${rule.path}` };
    }
  }

  return { ok: true, value: merged };
}

/** The template's list and the repository's, joined the way the rule says. */
function mergeArrays(
  base: readonly JsonValue[],
  override: readonly JsonValue[],
  strategy: string,
  deduplicate: boolean,
): JsonValue[] {
  const combined =
    strategy === 'append'
      ? [...base, ...override]
      : strategy === 'prepend'
        ? [...override, ...base]
        : [...override];
  if (!deduplicate) return combined;

  const kept: JsonValue[] = [];
  for (const value of combined) {
    if (!kept.some((one) => same(one, value))) kept.push(value);
  }

  return kept;
}

/*
 * A path names one place in a document: `$` for the whole of it, then a dot and
 * a key for each level below. A backslash escapes the character after it, so a
 * key that itself contains a dot is written `$.example\.com`. Spelled exactly
 * as `internal/orgsync/filemerge/path.go` spells it, including which shapes are
 * refused - a path that matches nothing and a path that is misspelled are the
 * same silence, and both are worth a reason.
 */
function parsePath(path: string): string[] | string {
  if (path === '') return 'A path cannot be empty';
  if (path[0] !== '$') return `Path ${path} does not start with $`;
  if (path.length === 1) return `Path ${path} names the whole document, which is never a list`;
  if (path[1] !== '.') return `Path ${path} needs a . after the $`;

  const keys: string[] = [];
  let current = '';
  const rest = path.slice(2);
  for (let index = 0; index < rest.length; index++) {
    const character = rest[index];
    if (character === '\\') {
      if (index + 1 === rest.length) return `Path ${path} ends in a \\, which escapes nothing`;
      index++;
      current += rest[index];
      continue;
    }
    if (character === '.') {
      keys.push(current);
      current = '';
      continue;
    }
    if (character === '[') {
      return `Path ${path} indexes a list, and a list's positions move when it is merged`;
    }
    current += character;
  }
  keys.push(current);

  if (keys.some((key) => key === '')) return `Path ${path} has an empty key`;

  return keys;
}

/** What a document holds at a path, or undefined where it holds nothing. */
function valueAt(document: JsonValue, keys: readonly string[]): JsonValue | undefined {
  let current: JsonValue = document;
  for (const key of keys.slice(0, -1)) {
    if (!isObject(current)) return undefined;
    current = current[key] ?? null;
  }
  if (!isObject(current)) return undefined;
  const last = keys[keys.length - 1] as string;

  return Object.prototype.hasOwnProperty.call(current, last) ? current[last] : undefined;
}

/**
 * Write a value at a path, reporting whether the path was there to write to.
 *
 * It never builds the levels above. A rule addresses a list the composed
 * document already has; creating the branches on the way to one it does not
 * would write a key nobody asked for into somebody's file.
 */
function setValueAt(document: JsonValue, keys: readonly string[], value: JsonValue): boolean {
  let current: JsonValue = document;
  for (const key of keys.slice(0, -1)) {
    if (!isObject(current)) return false;
    current = current[key] ?? null;
  }
  if (!isObject(current)) return false;
  put(current, keys[keys.length - 1] as string, value);

  return true;
}

/**
 * The smallest adjustment that turns `before` into `after`.
 *
 * `undefined` where they are already the same, so a caller can tell "nothing to
 * store" from "store an empty object". A key set to `null` in `after` returns
 * `'unsayable'`: RFC 7396 reads that as a removal, and writing a patch that
 * means something other than what somebody typed is worse than saying it cannot
 * be stored.
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
  for (const key of Object.keys(after)) {
    const value = after[key] as JsonValue;
    // The one value RFC 7396 cannot ask for: a `null` in a patch removes the
    // key rather than setting it to null. Already null is nothing to say.
    if (value === null) {
      if (key in before && before[key] === null) continue;

      return 'unsayable';
    }
    if (!Object.prototype.hasOwnProperty.call(before, key)) {
      put(patch, key, value);
      continue;
    }
    const inner = derivePatch(before[key] ?? null, value);
    if (inner === 'unsayable') return 'unsayable';
    if (inner !== undefined) put(patch, key, inner);
  }
  for (const key of Object.keys(before)) {
    if (!Object.prototype.hasOwnProperty.call(after, key)) put(patch, key, null);
  }

  return Object.keys(patch).length === 0 ? undefined : patch;
}

/** The adjustment that composes into what was typed, or the reason there is none. */
export type Derived = { ok: true; overrides: JsonValue } | { ok: false; reason: string };

/**
 * What has to be stored for this repository to end up with what was typed.
 *
 * Derived and then CHECKED: the candidate is composed again through
 * `composeFile` and compared with what the reader typed. Deriving alone is only
 * an inverse where the merge has one - a deduplicated append does not, since
 * the entry that was dropped could have come from either list - so a derivation
 * that cannot be proved is refused rather than stored. The check costs one more
 * compose of a file somebody is looking at.
 */
export function deriveOverrides(template: JsonValue, spec: MergeSpec, wanted: JsonValue): Derived {
  const patch =
    spec.strategy === 'shallow-merge'
      ? deriveShallow(template, wanted)
      : derivePatch(template, wanted);
  if (patch === 'unsayable') {
    return {
      ok: false,
      reason:
        'A merge cannot set a key to null - null is how it removes one - so this cannot be stored',
    };
  }

  const candidate = isObject(patch) ? copy(patch) : {};

  /* A rule's list is stored as what the repository contributes, not as what the
     file ends up with, so the derived patch's list has to have the template's
     share taken back off it. */
  for (const rule of spec.arrays ?? []) {
    const keys = parsePath(rule.path);
    if (typeof keys === 'string') return { ok: false, reason: keys };

    const result = valueAt(wanted, keys);
    if (result === undefined || !Array.isArray(result)) {
      return { ok: false, reason: `${rule.path} is no longer a list, so its rule cannot apply` };
    }
    const held = valueAt(template, keys);
    const base = Array.isArray(held) ? held : [];
    const share = contribution(base, result, rule.strategy);
    if (share === undefined) {
      return {
        ok: false,
        reason: `The list at ${rule.path} no longer ${
          rule.strategy === 'append' ? 'starts' : 'ends'
        } with the template's own entries, and this repository ${rule.strategy}s to that list`,
      };
    }
    if (!setValueAt(candidate, keys, share) && !plant(candidate, keys, share)) {
      return { ok: false, reason: `Nothing in the adjustment holds ${rule.path}` };
    }
  }

  const composed = composeFile(template, { ...spec, overrides: candidate });
  if (!composed.ok) return { ok: false, reason: composed.reason };
  if (!same(composed.value, wanted)) {
    return {
      ok: false,
      reason:
        'This cannot be stored as an adjustment: composing it again does not give what is written above',
    };
  }

  return { ok: true, overrides: candidate };
}

/** A shallow merge's patch: the top-level keys that differ, whole. */
function deriveShallow(before: JsonValue, after: JsonValue): JsonValue | 'unsayable' {
  if (!isObject(before) || !isObject(after)) return after;

  const patch: { [key: string]: JsonValue } = {};
  for (const key of Object.keys(after)) {
    const value = after[key] as JsonValue;
    if (value === null && !(key in before && before[key] === null)) return 'unsayable';
    if (!same(before[key] ?? undefined, value)) put(patch, key, value);
  }
  for (const key of Object.keys(before)) {
    if (!Object.prototype.hasOwnProperty.call(after, key)) put(patch, key, null);
  }

  return patch;
}

/** What the repository contributes to a combined list, given what the template gave. */
function contribution(
  base: readonly JsonValue[],
  result: readonly JsonValue[],
  strategy: string,
): JsonValue[] | undefined {
  if (strategy === 'append') {
    if (result.length < base.length) return undefined;
    if (!base.every((one, index) => same(one, result[index] as JsonValue))) return undefined;

    return result.slice(base.length);
  }
  if (strategy === 'prepend') {
    const at = result.length - base.length;
    if (at < 0) return undefined;
    if (!base.every((one, index) => same(one, result[at + index] as JsonValue))) return undefined;

    return result.slice(0, at);
  }

  return [...result];
}

/** Build the levels a rule's path needs, which only the adjustment may grow. */
function plant(document: JsonValue, keys: readonly string[], value: JsonValue): boolean {
  let current: JsonValue = document;
  for (const key of keys.slice(0, -1)) {
    if (!isObject(current)) return false;
    if (!isObject(current[key])) put(current, key, {});
    current = current[key] as JsonValue;
  }
  if (!isObject(current)) return false;
  put(current, keys[keys.length - 1] as string, value);

  return true;
}

function same(left: JsonValue | undefined, right: JsonValue | undefined): boolean {
  return canonical(left) === canonical(right);
}

/** One spelling per value, so two of them can be compared as text. */
function canonical(value: JsonValue | undefined): string {
  if (value === undefined) return ' ';
  if (isNumber(value)) return `#${value.rawJSON}`;
  if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`;
  if (isObject(value)) {
    return `{${Object.keys(value)
      .sort()
      .map((key) => `${JSON.stringify(key)}:${canonical(value[key] as JsonValue)}`)
      .join(',')}}`;
  }

  return JSON.stringify(value) ?? 'null';
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

  return Object.keys(patch).flatMap((key) => {
    const value = patch[key] as JsonValue;
    const path = prefix === '' ? key : `${prefix}.${key}`;

    return isObject(value) && Object.keys(value).length > 0 ? patchedKeys(value, path) : [path];
  });
}

/**
 * The lists the template and the adjustment both set.
 *
 * The one question a merge cannot answer on its own: two lists can be joined in
 * three ways and none of them is more correct than the others, so it is asked
 * where it arises rather than guessed. Returned as the path the stored array
 * rules are spelled with, escapes included, so a key holding a dot addresses
 * itself rather than two levels that are not there.
 */
export function sharedArrays(template: JsonValue, patch: JsonValue, prefix = '$'): string[] {
  if (!isObject(template) || !isObject(patch)) return [];

  return Object.keys(patch).flatMap((key) => {
    const value = patch[key] as JsonValue;
    const path = `${prefix}.${escapeKey(key)}`;
    const theirs = template[key];
    if (Array.isArray(value) && Array.isArray(theirs)) return [path];

    return sharedArrays(theirs ?? null, value, path);
  });
}

function escapeKey(key: string): string {
  return key.replace(/[\\.[]/g, (character) => `\\${character}`);
}

/** Whether a path is a file this can compose - which is JSON and nothing else yet. */
export function composable(path: string): boolean {
  return path.toLowerCase().endsWith('.json');
}

/**
 * The document, or nothing where it is not the JSON this composes.
 *
 * Numbers are kept as the digits they were written with. The service decodes
 * with `UseNumber` and writes the literal back, so a template holding `1.50` or
 * an identifier past 2^53 keeps it; parsing those into JavaScript numbers would
 * draw a file one digit different from the one the repository gets.
 */
export function parseJson(text: string): JsonValue | undefined {
  try {
    return JSON.parse(text, function (_key: string, value: unknown, context?: { source?: string }) {
      if (typeof value === 'number' && context?.source !== undefined) {
        return JSON.rawJSON(context.source);
      }

      return value;
    }) as JsonValue;
  } catch {
    return undefined;
  }
}

/**
 * Written back the way the service writes it.
 *
 * Keys in order, because the service decodes into a Go map and encodes that
 * back, and an encoder writes a map's keys sorted. Two spaces and a closing
 * newline for the same reason. A file whose keys are in the template's order is
 * not the file the repository is about to hold.
 */
export function formatJson(value: JsonValue): string {
  return `${JSON.stringify(sorted(value), null, 2)}\n`;
}

function sorted(value: JsonValue): JsonValue {
  if (Array.isArray(value)) return value.map(sorted);
  if (!isObject(value)) return value;
  const result: { [key: string]: JsonValue } = {};
  for (const key of Object.keys(value).sort()) put(result, key, sorted(value[key] as JsonValue));

  return result;
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
 *
 * The key on a line is READ as JSON rather than matched as text. It used to be
 * compared raw, so a key holding an escape - `"a\"b"`, or anything non-ASCII
 * that the encoder escapes - was compared against its parsed name and never
 * matched, and the line it names went unmarked with nothing to say it had.
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
      const key = /^\s*("(?:[^"\\]|\\.)*")\s*:/.exec(line);
      const named = key === null ? undefined : readKey(key[1] as string);
      if (depth === 1 && named !== undefined && wanted.has(named)) {
        marks.push(index + 1);
        if (after > depth) marking = depth;
      }
    }

    depth = after;
  });

  return marks;
}

/** The name a quoted key holds, with its escapes read. */
function readKey(quoted: string): string | undefined {
  try {
    const name: unknown = JSON.parse(quoted);

    return typeof name === 'string' ? name : undefined;
  } catch {
    return undefined;
  }
}
