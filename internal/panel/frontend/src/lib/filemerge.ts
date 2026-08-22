/**
 * The browser's copy of the merge the service performs, for previews: what a
 * repository ends up with, composed from the template and its adjustment.
 * JSON only - it is the format the templates that get adjusted are written
 * in, and the one the page can parse without shipping a parser.
 *
 * The vocabulary is `internal/orgsync/filemerge`'s: RFC 7396 with null
 * removing a key, `deep-merge` descending into objects, `shallow-merge`
 * replacing top-level keys, and array rules deciding what happens to a list
 * both sides set - without one, it is replaced.
 */

export interface ArrayRule {
  path: string;
  strategy: 'replace' | 'append' | 'prepend' | string;
}

export interface FileMergeSpec {
  path?: string;
  strategy?: string;
  overrides?: Record<string, unknown>;
  arrays?: ArrayRule[];
  [key: string]: unknown;
}

type JsonValue = unknown;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function clone<T>(value: T): T {
  return value === undefined ? value : (JSON.parse(JSON.stringify(value)) as T);
}

/** The JSONPath spelling the service stores for an array rule. */
export function arrayRulePath(keys: readonly string[]): string {
  const escaped = keys.map((key) =>
    key.replaceAll('\\', '\\\\').replaceAll('.', '\\.').replaceAll('[', '\\['),
  );
  return `$.${escaped.join('.')}`;
}

function ruleFor(rules: readonly ArrayRule[], at: readonly string[]): ArrayRule | undefined {
  const path = arrayRulePath(at);
  return rules.find((rule) => rule.path === path);
}

function mergeArrays(base: unknown[], patch: unknown[], rule: ArrayRule | undefined): unknown[] {
  if (rule?.strategy === 'append') return [...clone(base), ...clone(patch)];
  if (rule?.strategy === 'prepend') return [...clone(patch), ...clone(base)];
  return clone(patch);
}

function mergeDeep(
  base: JsonValue,
  patch: JsonValue,
  rules: readonly ArrayRule[],
  at: readonly string[],
): JsonValue {
  if (Array.isArray(base) && Array.isArray(patch)) {
    return mergeArrays(base, patch, ruleFor(rules, at));
  }
  if (!isRecord(base) || !isRecord(patch)) return clone(patch);
  const merged: Record<string, unknown> = { ...clone(base) };
  for (const [key, value] of Object.entries(patch)) {
    if (value === null) delete merged[key];
    else merged[key] = mergeDeep(merged[key], value, rules, [...at, key]);
  }
  return merged;
}

function mergeShallow(base: JsonValue, patch: JsonValue, rules: readonly ArrayRule[]): JsonValue {
  if (!isRecord(base) || !isRecord(patch)) return clone(patch);
  const merged: Record<string, unknown> = { ...clone(base) };
  for (const [key, value] of Object.entries(patch)) {
    if (value === null) delete merged[key];
    else if (Array.isArray(merged[key]) && Array.isArray(value)) {
      merged[key] = mergeArrays(merged[key], value, ruleFor(rules, [key]));
    } else merged[key] = clone(value);
  }
  return merged;
}

/**
 * The composed copy, or null where this preview cannot honestly draw one -
 * a template that is not JSON, or a strategy this copy does not speak.
 */
export function mergedPreview(templateText: string, merge: FileMergeSpec): string | null {
  const strategy = merge.strategy ?? 'deep-merge';
  if (strategy !== 'deep-merge' && strategy !== 'shallow-merge') return null;
  let template: JsonValue;
  try {
    template = JSON.parse(stripJsonComments(templateText));
  } catch {
    return null;
  }
  const rules = merge.arrays ?? [];
  const overrides = merge.overrides ?? {};
  const merged =
    strategy === 'deep-merge'
      ? mergeDeep(template, overrides, rules, [])
      : mergeShallow(template, overrides, rules);
  return JSON.stringify(merged, null, 2);
}

/**
 * Comments survive the service's own JSON reading, so the preview strips
 * them the same way rather than refusing a template a human annotated.
 */
function stripJsonComments(text: string): string {
  return text.replace(/^\s*\/\/.*$/gm, '');
}

/**
 * The template in the preview's own formatting, so a diff between the two
 * marks what an adjustment changed rather than what a re-print reflowed.
 */
export function normalizedJson(text: string): string | null {
  try {
    return JSON.stringify(JSON.parse(stripJsonComments(text)), null, 2);
  } catch {
    return null;
  }
}

/** The keys an adjustment writes, split the way the page speaks about them. */
export function mergeSummary(merge: FileMergeSpec): {
  changed: string[];
  removed: string[];
  listed: Array<{ key: string; strategy: string; entries: number }>;
} {
  const overrides = merge.overrides ?? {};
  const rules = merge.arrays ?? [];
  const changed: string[] = [];
  const removed: string[] = [];
  const listed: Array<{ key: string; strategy: string; entries: number }> = [];
  for (const [key, value] of Object.entries(overrides)) {
    const rule = ruleFor(rules, [key]);
    if (value === null) removed.push(key);
    else if (Array.isArray(value) && rule !== undefined && rule.strategy !== 'replace') {
      listed.push({ key, strategy: rule.strategy, entries: value.length });
    } else changed.push(key);
  }
  return { changed, removed, listed };
}
