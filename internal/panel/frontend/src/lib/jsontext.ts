/**
 * The merge over the template's own bytes. `filemerge.ts` answers what a
 * repository ends up with as parsed values; this module uses jsonc-parser to
 * produce a responsive local editing aid. The backend render endpoint owns
 * the exact bytes shown as final output. This module also runs the other way:
 * given the template and an edited copy, it derives the RFC 7396 override
 * that turns one into the other - which is what makes the composed copy an
 * editable surface rather than a printout.
 *
 * jsonc-parser deliberately owns serialization here. Keeping those rules out
 * of browser code prevents the editing aid from becoming a second formatter.
 */
import {
  applyEdits,
  modify,
  parseTree,
  type FormattingOptions,
  type Node,
  type ParseError,
} from 'jsonc-parser';

import { arrayRulePath, type ArrayRule, type FileMergeSpec } from './filemerge';
import type { MergeSpec } from './merge';

type Segments = Array<string | number>;
type ObjectPath = string[];
const invalidJSON = Symbol('invalid JSON');

function emptyRecord(): Record<string, unknown> {
  return Object.create(null) as Record<string, unknown>;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function ownValue(record: unknown, key: string): unknown {
  return isRecord(record) && Object.prototype.hasOwnProperty.call(record, key)
    ? record[key]
    : undefined;
}

function ruleFor(rules: readonly ArrayRule[], at: readonly string[]): ArrayRule | undefined {
  const path = arrayRulePath(at);
  return rules.find((rule) => rule.path === path);
}

function formatting(text: string): { formattingOptions: FormattingOptions } {
  const spaces = /^( +)\S/mu.exec(text)?.[1].length ?? 2;

  return {
    formattingOptions: {
      insertSpaces: !/^\t/mu.test(text),
      tabSize: spaces,
      eol: text.includes('\r\n') ? '\r\n' : '\n',
      keepLines: true,
    },
  };
}

function parseLoose(text: string): unknown {
  const errors: ParseError[] = [];
  const root = parseTree(text, errors, { allowTrailingComma: true });
  if (errors.length > 0 || root === undefined) return undefined;
  const value = nodeValue(root);

  return value === invalidJSON ? undefined : value;
}

function nodeValue(node: Node): unknown | typeof invalidJSON {
  if (node.type === 'array') {
    const value: unknown[] = [];
    for (const child of node.children ?? []) {
      const held = nodeValue(child);
      if (held === invalidJSON) return invalidJSON;
      value.push(held);
    }
    return value;
  }
  if (node.type !== 'object') return node.value;

  const value = emptyRecord();
  for (const property of node.children ?? []) {
    const name = property.children?.[0]?.value;
    const held = property.children?.[1];
    if (typeof name !== 'string' || held === undefined || Object.hasOwn(value, name)) {
      return invalidJSON;
    }
    const decoded = nodeValue(held);
    if (decoded === invalidJSON) return invalidJSON;
    value[name] = decoded;
  }

  return value;
}

interface Op {
  path: Segments;
  value: unknown;
  /** An entry slotted into a list, leaving the entries around it their bytes. */
  insert?: boolean;
}

/* An appended or prepended list lands as per-entry insertions, so the
   template's own entries keep their bytes and the gutter marks only what
   the adjustment added. A replaced list is set whole. */
function listOps(
  path: Segments,
  held: unknown[],
  value: unknown[],
  rule: ArrayRule | undefined,
  deduplicate: boolean,
): Op[] {
  if (deduplicate) return deduplicatedListOps(path, held, value, rule);
  if (rule?.strategy === 'append') {
    return value.map((entry, i) => ({
      path: [...path, held.length + i],
      value: entry,
      insert: true,
    }));
  }
  if (rule?.strategy === 'prepend') {
    return value.map((entry, i) => ({ path: [...path, i], value: entry, insert: true }));
  }
  return [{ path, value }];
}

interface ListEntry {
  value: unknown;
  source: 'base' | 'override';
  index: number;
}

function deduplicatedListOps(
  path: Segments,
  held: unknown[],
  value: unknown[],
  rule?: ArrayRule,
): Op[] {
  if (rule?.strategy !== 'append' && rule?.strategy !== 'prepend') {
    return [{ path, value: uniqueValues(value) }];
  }
  const base = held.map((entry, index): ListEntry => ({ value: entry, source: 'base', index }));
  const override = value.map((entry, index): ListEntry => ({
    value: entry,
    source: 'override',
    index,
  }));
  const combined = rule.strategy === 'append' ? [...base, ...override] : [...override, ...base];
  const kept: ListEntry[] = [];
  for (const candidate of combined) {
    if (!kept.some((entry) => deepEqual(entry.value, candidate.value))) kept.push(candidate);
  }
  const keptBase = new Set(
    kept.filter((entry) => entry.source === 'base').map((entry) => entry.index),
  );
  const removals: Op[] = [];
  for (let index = held.length - 1; index >= 0; index -= 1) {
    if (!keptBase.has(index)) removals.push({ path: [...path, index], value: undefined });
  }
  const additions = kept.filter((entry) => entry.source === 'override');
  const start = rule.strategy === 'append' ? keptBase.size : 0;
  const insertions = additions.map((entry, index): Op => ({
    path: [...path, start + index],
    value: entry.value,
    insert: true,
  }));

  return [...removals, ...insertions];
}

function uniqueValues(values: unknown[]): unknown[] {
  const kept: unknown[] = [];
  for (const value of values) {
    if (!kept.some((entry) => deepEqual(entry, value))) kept.push(value);
  }

  return kept;
}

function opsDeep(
  base: unknown,
  patch: Record<string, unknown>,
  at: ObjectPath,
  rules: readonly ArrayRule[],
  deduplicate: boolean,
): Op[] {
  const out: Op[] = [];
  for (const key of Object.keys(patch).sort()) {
    const value = patch[key];
    const path = [...at, key];
    const held = ownValue(base, key);
    if (value === null) {
      out.push({ path, value: undefined });
    } else if (isRecord(value) && isRecord(held)) {
      out.push(...opsDeep(held, value, path, rules, deduplicate));
    } else if (Array.isArray(value) && Array.isArray(held)) {
      out.push(...listOps(path, held, value, ruleFor(rules, path), deduplicate));
    } else {
      out.push({ path, value });
    }
  }
  return out;
}

function opsShallow(
  base: unknown,
  patch: Record<string, unknown>,
  rules: readonly ArrayRule[],
  deduplicate: boolean,
): Op[] {
  const out: Op[] = [];
  for (const key of Object.keys(patch).sort()) {
    const value = patch[key];
    const held = ownValue(base, key);
    if (value === null) {
      out.push({ path: [key], value: undefined });
    } else if (Array.isArray(value) && Array.isArray(held)) {
      out.push(...listOps([key], held, value, ruleFor(rules, [key]), deduplicate));
    } else {
      out.push({ path: [key], value });
    }
  }
  return out;
}

/**
 * A local composed copy. It is deliberately not the final-byte authority.
 * Null means the template is not JSON or the strategy is not one this copy
 * speaks.
 */
export function composeMergedText(
  templateText: string,
  merge: FileMergeSpec | MergeSpec,
): string | null {
  const strategy = merge.strategy ?? 'deep-merge';
  if (strategy !== 'deep-merge' && strategy !== 'shallow-merge') return null;
  const template = parseLoose(templateText);
  if (template === undefined) return null;
  const rules = merge.arrays ?? [];
  const overrides = merge.overrides ?? {};
  if (!isRecord(overrides)) return null;
  const deduplicate = merge.deduplicate === true;
  const ops =
    strategy === 'deep-merge'
      ? opsDeep(template, overrides, [], rules, deduplicate)
      : opsShallow(template, overrides, rules, deduplicate);
  let text = templateText;
  for (const op of ops) {
    const options =
      op.insert === true
        ? { ...formatting(templateText), isArrayInsertion: true }
        : formatting(templateText);
    text = applyEdits(text, modify(text, op.path, op.value, options));
  }
  return text;
}

/* ---------- The other direction: edited copy back to an override ---------- */

/**
 * A list both sides set, where the edited copy still contains the template's
 * entries in order - so the difference is expressible as more than a
 * wholesale replacement, and the one question a merge cannot answer itself
 * has answers worth offering.
 */
export interface ListQuestion {
  path: string;
  canAppend: boolean;
  canPrepend: boolean;
  chosen: 'append' | 'prepend' | 'replace';
}

export interface DerivedMerge {
  /** Empty means the edited copy is the template - no override left. */
  overrides: Record<string, unknown>;
  arrays: ArrayRule[];
  questions: ListQuestion[];
}

function deepEqual(a: unknown, b: unknown): boolean {
  if (Object.is(a, b)) return true;
  if (Array.isArray(a) && Array.isArray(b)) {
    return a.length === b.length && a.every((held, i) => deepEqual(held, b[i]));
  }
  if (isRecord(a) && isRecord(b)) {
    const keys = Object.keys(a);
    return keys.length === Object.keys(b).length && keys.every((k) => deepEqual(a[k], b[k]));
  }
  return false;
}

function isPrefix(prefix: unknown[], whole: unknown[]): boolean {
  return prefix.length < whole.length && prefix.every((held, i) => deepEqual(held, whole[i]));
}

function isSuffix(suffix: unknown[], whole: unknown[]): boolean {
  const from = whole.length - suffix.length;
  return from > 0 && suffix.every((held, i) => deepEqual(held, whole[from + i]));
}

function diffList(
  base: unknown[],
  next: unknown[],
  at: string[],
  prior: readonly ArrayRule[],
  out: DerivedMerge,
): void {
  const canAppend = isPrefix(base, next);
  const canPrepend = isSuffix(base, next);
  const path = arrayRulePath(at);
  const asked = ruleFor(prior, at)?.strategy;
  const chosen =
    asked === 'append' && canAppend
      ? 'append'
      : asked === 'prepend' && canPrepend
        ? 'prepend'
        : 'replace';
  const value =
    chosen === 'append'
      ? next.slice(base.length)
      : chosen === 'prepend'
        ? next.slice(0, next.length - base.length)
        : next;
  setAt(out.overrides, at, value);
  if (chosen !== 'replace') out.arrays.push({ path, strategy: chosen });
  if (canAppend || canPrepend) out.questions.push({ path, canAppend, canPrepend, chosen });
}

/** Writes a value into the nested override document at its object-key path. */
function setAt(overrides: Record<string, unknown>, parts: string[], value: unknown): void {
  let held = overrides;
  for (const part of parts.slice(0, -1)) {
    const next = held[part];
    if (isRecord(next)) held = next;
    else {
      const fresh = emptyRecord();
      held[part] = fresh;
      held = fresh;
    }
  }
  const key = parts[parts.length - 1];
  if (key !== undefined) held[key] = value;
}

function diffDeep(
  base: Record<string, unknown>,
  next: Record<string, unknown>,
  at: string[],
  prior: readonly ArrayRule[],
  out: DerivedMerge,
): void {
  for (const key of Object.keys(base)) {
    if (!Object.hasOwn(next, key)) setAt(out.overrides, [...at, key], null);
  }
  for (const [key, value] of Object.entries(next)) {
    const path = [...at, key];
    const held = base[key];
    if (deepEqual(held, value)) continue;
    if (isRecord(held) && isRecord(value)) diffDeep(held, value, path, prior, out);
    else if (Array.isArray(held) && Array.isArray(value)) diffList(held, value, path, prior, out);
    else setAt(out.overrides, path, value);
  }
}

function diffShallow(
  base: Record<string, unknown>,
  next: Record<string, unknown>,
  prior: readonly ArrayRule[],
  out: DerivedMerge,
): void {
  for (const key of Object.keys(base)) {
    if (!Object.hasOwn(next, key)) out.overrides[key] = null;
  }
  for (const [key, value] of Object.entries(next)) {
    const held = base[key];
    if (deepEqual(held, value)) continue;
    if (Array.isArray(held) && Array.isArray(value)) diffList(held, value, [key], prior, out);
    else out.overrides[key] = value;
  }
}

/**
 * The override that turns the template into the edited copy, under the given
 * strategy. `prior` carries the list answers already given - a stored rule,
 * or one chosen on an ask card - and a rule that no longer fits the edit
 * (the template's entries are gone from the list) falls back to replace.
 * Null where either side is not JSON, or not an object.
 */
export function deriveMerge(
  templateText: string,
  editedText: string,
  strategy: string,
  prior: readonly ArrayRule[],
): DerivedMerge | null {
  if (strategy !== 'deep-merge' && strategy !== 'shallow-merge') return null;
  const base = parseLoose(templateText);
  const next = parseLoose(editedText);
  if (!isRecord(base) || !isRecord(next)) return null;
  const out: DerivedMerge = { overrides: emptyRecord(), arrays: [], questions: [] };
  if (strategy === 'deep-merge') diffDeep(base, next, [], prior, out);
  else diffShallow(base, next, prior, out);
  return out;
}
