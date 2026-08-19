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

/**
 * A raw-JSON box, which is an object to `typeof` and a number to a reader.
 *
 * Guarded, because everything in this module goes through here - `isObject`
 * calls it, and every entry point calls that. On an engine without the raw-JSON
 * proposal an unguarded call is a `TypeError` from the first merge rather than
 * a panel that composes with ordinary numbers, which is what the absence should
 * cost: digits beyond what a double holds, and nothing else.
 */
function isNumber(value: unknown): value is JsonNumber {
  return typeof JSON.isRawJSON === 'function' && JSON.isRawJSON(value);
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

/**
 * Read a key without going up the prototype chain.
 *
 * The other half of `put`. `record.__proto__` is not a missing key - it answers
 * `Object.prototype`, which is an object, so walking a path through a level the
 * document does not have arrived at the prototype every object shares and wrote
 * onto it: `$.__proto__.list` gave every object in the page a `list`. Every
 * step through a document goes through here, so there is one rule rather than a
 * guard at whichever call somebody remembered.
 */
function own(record: { [key: string]: JsonValue }, key: string): JsonValue | undefined {
  return Object.prototype.hasOwnProperty.call(record, key) ? record[key] : undefined;
}

/** A shallow copy that keeps a `__proto__` key a key, for the same reason. */
function copy(source: { [key: string]: JsonValue }): { [key: string]: JsonValue } {
  const result: { [key: string]: JsonValue } = {};
  for (const key of Object.keys(source)) put(result, key, source[key] as JsonValue);

  return result;
}

/**
 * The template with its adjustment merged in, one key at a time.
 *
 * `deep` is the only thing the two strategies disagree about, and it is one
 * line: a deep merge recurses into a key both sides hold, a shallow one
 * replaces it. Everything else - a non-object patch replacing whatever it
 * lands on, a null removing a key, `__proto__` staying an ordinary key - is
 * the same rule, and was written twice.
 *
 * A null removes a key under both. Below the top level a shallow merge never
 * looks, so a null there is a null value rather than a removal - which falls
 * out of not recursing rather than needing a rule of its own.
 */
function merge(target: JsonValue, patch: JsonValue, deep: boolean): JsonValue {
  if (!isObject(patch)) return patch;

  const result = isObject(target) ? copy(target) : {};
  for (const key of Object.keys(patch)) {
    const value = patch[key] as JsonValue;
    if (value === null) {
      delete result[key];
      continue;
    }
    put(result, key, deep ? merge(own(result, key) ?? null, value, true) : value);
  }

  return result;
}

/** A deep merge, key by key all the way down. RFC 7396. */
export function mergePatch(target: JsonValue, patch: JsonValue): JsonValue {
  return merge(target, patch, true);
}

/** A shallow merge replaces top-level keys rather than merging into them. */
function mergeShallow(target: JsonValue, patch: JsonValue): JsonValue {
  return merge(target, patch, false);
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
  return (
    (spec.strategy ?? '') === '' &&
    !adjusts(spec) &&
    (spec.arrays ?? []).length === 0 &&
    spec.deduplicate !== true &&
    (spec.sections ?? []).length === 0
  );
}

/** Overrides that say something, which absent, null and `{}` all do not. */
function adjusts(spec: MergeSpec): boolean {
  const overrides = spec.overrides;

  return (
    overrides !== undefined &&
    overrides !== null &&
    (!isObject(overrides) || Object.keys(overrides).length > 0)
  );
}

/** The list rules the service knows, which is all three of them. */
export const ARRAY_STRATEGIES = ['replace', 'append', 'prepend'] as const;

export type ArrayStrategy = (typeof ARRAY_STRATEGIES)[number];

/**
 * One of the three, or nothing.
 *
 * A control hands back the `string` it is declared to hand back, and a rule
 * holds one of three words - so somewhere the two have to meet. Here rather
 * than at a cast, because a cast would let a fourth word through silently and
 * the engine refuses the whole file sync over it.
 */
export function asArrayStrategy(value: string): ArrayStrategy | undefined {
  return (ARRAY_STRATEGIES as readonly string[]).includes(value)
    ? (value as ArrayStrategy)
    : undefined;
}

/**
 * A merge nobody should be able to configure, for the file it applies to - or
 * nothing, where the spec is one the service would run.
 *
 * `filemerge.Spec.Validate` clause for clause. It exists because the two halves
 * refuse in different places: the service refuses at plan time and the reader
 * never sees the plan, so a spec it will not run has to be refused here too or
 * the panel draws a composed file for a merge that is never going to happen.
 * `markdown` on a JSON path was the one that showed - it fell through to a deep
 * merge and drew a perfectly ordinary file.
 *
 * The file decides most of it: a strategy is only meaningful for the sort of
 * document it edits.
 */
export function validateSpec(path: string, spec: MergeSpec): string | undefined {
  if (!composable(path)) {
    // Narrower than the service on purpose. `Validate` also accepts YAML and
    // Markdown; this composes JSON, and the caller draws "cannot be composed
    // here" rather than a file. Refused rather than assumed so a future caller
    // that forgets the gate cannot get a JSON merge run over a YAML document.
    return `${path} is not JSON, which is all this composes`;
  }

  const strategy =
    spec.strategy === undefined || spec.strategy === '' ? 'deep-merge' : spec.strategy;
  if (strategy === 'markdown') {
    return `${path} is not Markdown, so it cannot be merged by its headings`;
  }
  if (strategy !== 'deep-merge' && strategy !== 'shallow-merge') {
    return `Unknown strategy ${JSON.stringify(strategy)}`;
  }

  if ((spec.sections ?? []).length > 0) {
    return 'Sections edit Markdown headings, and this file has none';
  }

  const overrides = spec.overrides;
  if (overrides !== undefined && overrides !== null && !isObject(overrides)) {
    return 'The overrides are not an object';
  }

  const rules = spec.arrays ?? [];
  if (spec.deduplicate === true && rules.length === 0) {
    return (
      'Nothing is deduplicated without a list rule, because a list with no ' +
      'rule is replaced whole'
    );
  }

  if (!adjusts(spec) && rules.length === 0) {
    return 'Nothing is merged without overrides or a list rule';
  }

  const seen = new Set<string>();
  for (const [index, rule] of rules.entries()) {
    if (!(ARRAY_STRATEGIES as readonly string[]).includes(rule.strategy)) {
      return `List rule ${index + 1} has unknown strategy ${JSON.stringify(rule.strategy)}`;
    }
    if (seen.has(rule.path)) return `${rule.path} has two list rules`;
    seen.add(rule.path);
  }

  return undefined;
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
export function composeFile(path: string, template: JsonValue, spec: MergeSpec): Composed {
  const refused = validateSpec(path, spec);
  if (refused !== undefined) return { ok: false, reason: refused };

  const overrides = spec.overrides ?? {};
  const merged =
    spec.strategy === 'shallow-merge'
      ? mergeShallow(template, overrides)
      : mergePatch(template, overrides);

  for (const rule of spec.arrays ?? []) {
    const keys = keysFor(spec, rule.path);
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

/**
 * The keys one rule's path names, for the merge the rule belongs to.
 *
 * A shallow merge replaces a top-level key whole, so nothing below one is ever
 * merged and a rule pointing there describes work that cannot happen. The
 * service refuses that spec rather than applying it (`Spec.validateArrays`), so
 * composing it here would draw a file no repository is ever going to hold - and
 * the composed value would alias the adjustment it came from, which is how one
 * preview grew the list every time it was drawn.
 */
function keysFor(spec: MergeSpec, path: string): string[] | string {
  const keys = parsePath(path);
  if (typeof keys === 'string') return keys;
  if (spec.strategy === 'shallow-merge' && keys.length > 1) {
    return `${path} is below the top level, and a shallow merge replaces top-level keys whole`;
  }

  return keys;
}

/**
 * The object a path's last key lives in, or undefined where the path does not
 * lead to one.
 *
 * `creating` is the whole difference between the two ways this document is
 * written. A rule addresses a list the composed document already has, so a
 * write for one builds nothing on the way - creating the branches to a list it
 * does not have would put a key nobody asked for into somebody's file. An
 * override is somebody typing a value that is not there yet, so that one does
 * build them.
 */
function parentAt(
  document: JsonValue,
  keys: readonly string[],
  creating: boolean,
): { [key: string]: JsonValue } | undefined {
  let current: JsonValue = document;
  for (const key of keys.slice(0, -1)) {
    if (!isObject(current)) return undefined;
    if (creating && !isObject(own(current, key))) put(current, key, {});
    current = own(current, key) ?? null;
  }

  return isObject(current) ? current : undefined;
}

/** The last key of a path - the one the walk stops before. */
function lastKey(keys: readonly string[]): string {
  return keys[keys.length - 1] as string;
}

/** What a document holds at a path, or undefined where it holds nothing. */
function valueAt(document: JsonValue, keys: readonly string[]): JsonValue | undefined {
  const parent = parentAt(document, keys, false);

  return parent === undefined ? undefined : own(parent, lastKey(keys));
}

/** Write a value at a path, reporting whether the path was there to write to. */
function setValueAt(document: JsonValue, keys: readonly string[], value: JsonValue): boolean {
  const parent = parentAt(document, keys, false);
  if (parent === undefined) return false;
  put(parent, lastKey(keys), value);

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
      if (own(before, key) === null) continue;

      return 'unsayable';
    }
    if (!Object.prototype.hasOwnProperty.call(before, key)) {
      put(patch, key, value);
      continue;
    }
    const inner = derivePatch(own(before, key) ?? null, value);
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
export function deriveOverrides(
  path: string,
  template: JsonValue,
  spec: MergeSpec,
  wanted: JsonValue,
): Derived {
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
    const keys = keysFor(spec, rule.path);
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
    // `plant` alone: it succeeds everywhere `setValueAt` does and builds the
    // levels besides, so trying the stricter one first only ever wrote the same
    // value to the same place. It fails on one thing - an adjustment that is not
    // an object at all - which is what this reason is about.
    if (!plant(candidate, keys, share)) {
      return { ok: false, reason: `Nothing in the adjustment holds ${rule.path}` };
    }
  }

  const composed = composeFile(path, template, { ...spec, overrides: candidate });
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
    if (value === null && own(before, key) !== null) return 'unsayable';
    if (!same(own(before, key), value)) put(patch, key, value);
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
  const parent = parentAt(document, keys, true);
  if (parent === undefined) return false;
  put(parent, lastKey(keys), value);

  return true;
}

function same(left: JsonValue | undefined, right: JsonValue | undefined): boolean {
  return canonical(left) === canonical(right);
}

/** One spelling per value, so two of them can be compared as text. */
function canonical(value: JsonValue | undefined): string {
  // A byte no JSON text can hold, so no real value canonicalises to it. Spelled
  // as an escape rather than typed: a raw NUL makes this module binary to grep,
  // diff and every other text tool, and is invisible in an editor.
  if (value === undefined) return '\u0000';
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
    const theirs = own(template, key);
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
      if (
        typeof value === 'number' &&
        context?.source !== undefined &&
        typeof JSON.rawJSON === 'function'
      ) {
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
 *
 * Emitted here rather than handed to `JSON.stringify`, because sorting the keys
 * into a new object does not decide the order they come out in: JavaScript
 * enumerates an object's INTEGER-LIKE own keys first and in numeric order,
 * whatever order they were inserted in. So a document holding `"2"` and `"10"`
 * came out `2, 10` where Go, sorting the same keys as strings, writes `10, 2` -
 * and every line between them read as changed.
 */
export function formatJson(value: JsonValue): string {
  return `${emit(value, '')}\n`;
}

/** One value, written the way Go's encoder writes it at this indent. */
function emit(value: JsonValue, indent: string): string {
  if (isNumber(value)) return value.rawJSON;
  if (Array.isArray(value)) {
    if (value.length === 0) return '[]';
    const inner = indent + '  ';

    return `[\n${value.map((one) => inner + emit(one, inner)).join(',\n')}\n${indent}]`;
  }
  if (!isObject(value)) return jsonString(value);

  const keys = Object.keys(value).sort();
  if (keys.length === 0) return '{}';
  const inner = indent + '  ';

  return `{\n${keys
    .map((key) => `${inner}${quoted(key)}: ${emit(value[key] as JsonValue, inner)}`)
    .join(',\n')}\n${indent}}`;
}

/** A scalar. Only a string needs deciding; the rest are one spelling each. */
function jsonString(value: JsonValue): string {
  return typeof value === 'string' ? quoted(value) : (JSON.stringify(value) ?? 'null');
}

/**
 * A JSON string spelled the way Go spells it.
 *
 * Measured against the service's own encoder rather than assumed, because the
 * assumption was wrong twice: the two agree on `\b`, `\f` and `\u0000`, and
 * the service encodes with `SetEscapeHTML(false)` so `<`, `>` and `&` are left
 * alone as well. The one difference is U+2028 and U+2029, which Go escapes and
 * `JSON.stringify` writes as themselves - so everything goes through
 * `JSON.stringify` and only those two are rewritten afterwards, which also
 * means no other escape can drift out of step with it.
 */
function quoted(value: string): string {
  return JSON.stringify(value).replaceAll('\u2028', '\\u2028').replaceAll('\u2029', '\\u2029');
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
