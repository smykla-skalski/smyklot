/**
 * The merge over the template's own bytes. `filemerge.ts` answers what a
 * repository ends up with as parsed values; this module answers it as text,
 * editing the template in place so its comments and hand formatting survive
 * everywhere an adjustment did not reach. It also runs the other way:
 * given the template and an edited copy, it derives the RFC 7396 override
 * that turns one into the other - which is what makes the composed copy an
 * editable surface rather than a printout.
 *
 * One known ceiling, inherited from jsonc-parser: removing a key reformats
 * its immediate neighbours and takes the key's own leading comment with it.
 * Everything else is a local edit.
 */
import {
  applyEdits,
  findNodeAtLocation,
  modify,
  parse,
  parseTree,
  type ParseError,
} from 'jsonc-parser';

import { arrayRulePath, type ArrayRule, type FileMergeSpec } from './filemerge';

type Segments = Array<string | number>;
type ObjectPath = string[];

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function ruleFor(rules: readonly ArrayRule[], at: readonly string[]): ArrayRule | undefined {
  const path = arrayRulePath(at);
  return rules.find((rule) => rule.path === path);
}

function formatting(text: string): { formattingOptions: object } {
  return {
    formattingOptions: { insertSpaces: !/^\t/mu.test(text), tabSize: 2, keepLines: true },
  };
}

function parseLoose(text: string): unknown {
  const errors: ParseError[] = [];
  const value: unknown = parse(text, errors, { allowTrailingComma: true });
  return errors.length > 0 ? undefined : value;
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
function listOps(path: Segments, held: unknown[], value: unknown[], rule?: ArrayRule): Op[] {
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

function opsDeep(
  base: unknown,
  patch: Record<string, unknown>,
  at: ObjectPath,
  rules: readonly ArrayRule[],
): Op[] {
  const out: Op[] = [];
  for (const [key, value] of Object.entries(patch)) {
    const path = [...at, key];
    const held = isRecord(base) ? base[key] : undefined;
    if (value === null) {
      out.push({ path, value: undefined });
    } else if (isRecord(value) && isRecord(held)) {
      out.push(...opsDeep(held, value, path, rules));
    } else if (Array.isArray(value) && Array.isArray(held)) {
      out.push(...listOps(path, held, value, ruleFor(rules, path)));
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
): Op[] {
  const out: Op[] = [];
  for (const [key, value] of Object.entries(patch)) {
    const held = isRecord(base) ? base[key] : undefined;
    if (value === null) {
      out.push({ path: [key], value: undefined });
    } else if (Array.isArray(value) && Array.isArray(held)) {
      out.push(...listOps([key], held, value, ruleFor(rules, [key])));
    } else {
      out.push({ path: [key], value });
    }
  }
  return out;
}

/**
 * The composed copy as text: the template with the adjustment written into
 * it, everything untouched by the adjustment byte-identical. Null where the
 * template is not JSON or the strategy is not one this copy speaks.
 */
export function composeMergedText(templateText: string, merge: FileMergeSpec): string | null {
  const strategy = merge.strategy ?? 'deep-merge';
  if (strategy !== 'deep-merge' && strategy !== 'shallow-merge') return null;
  const template = parseLoose(templateText);
  if (template === undefined) return null;
  const rules = merge.arrays ?? [];
  const overrides = merge.overrides ?? {};
  const ops =
    strategy === 'deep-merge'
      ? opsDeep(template, overrides, [], rules)
      : opsShallow(template, overrides, rules);
  let text = templateText;
  for (const op of ops) {
    text =
      op.insert === true
        ? spliceListEntry(text, op.path, op.value, templateText)
        : setValue(text, op.path, op.value, templateText);
  }
  return text;
}

/**
 * A value written over an existing single-line one keeps the line's shape:
 * `"schedule": ["* 4 * * 6"]` replaced stays one line, the way a hand would
 * write it. Everything else - new keys, multiline values, removals - goes
 * through jsonc-parser's formatter.
 */
function setValue(text: string, at: Segments, value: unknown, templateText: string): string {
  if (value !== undefined) {
    const root = parseTree(text);
    const node = root === undefined ? undefined : findNodeAtLocation(root, at);
    if (node !== undefined && !text.slice(node.offset, node.offset + node.length).includes('\n')) {
      return (
        text.slice(0, node.offset) + compactPrint(value) + text.slice(node.offset + node.length)
      );
    }
  }
  return applyEdits(text, modify(text, at, value, formatting(templateText)));
}

/** JSON.stringify, with the one-space seams a hand puts after , and : . */
function compactPrint(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(compactPrint).join(', ')}]`;
  if (isRecord(value)) {
    const inner = Object.entries(value)
      .map(([key, held]) => `${JSON.stringify(key)}: ${compactPrint(held)}`)
      .join(', ');
    return `{ ${inner} }`;
  }
  return JSON.stringify(value);
}

/**
 * One entry slotted into a list by hand, because jsonc-parser's own array
 * insertion reformats the whole list - which would put gutter bars on the
 * template's own entries. This touches nothing but the seam: a compact list
 * stays compact, a multiline list gains lines shaped like its neighbours'.
 */
function spliceListEntry(text: string, at: Segments, value: unknown, templateText: string): string {
  const arrayPath = at.slice(0, -1);
  const index = at[at.length - 1];
  const root = parseTree(text);
  const node = root === undefined ? undefined : findNodeAtLocation(root, arrayPath);
  if (typeof index !== 'number' || node === undefined || node.type !== 'array') {
    // No list to slot into after all - fall back to the formatter's set.
    return applyEdits(
      text,
      modify(text, at, value, { ...formatting(templateText), isArrayInsertion: true }),
    );
  }
  const children = node.children ?? [];
  const body = text.slice(node.offset, node.offset + node.length);
  const compact = !body.includes('\n');
  /* A new entry is written the way its neighbours are: one line beside
     one-line entries, spread only where the list already spreads. */
  const neighboursCompact = children.every(
    (child) => !text.slice(child.offset, child.offset + child.length).includes('\n'),
  );
  const indentUnit = /^\t/mu.test(templateText) ? '\t' : '  ';
  const lineStart = text.lastIndexOf('\n', node.offset) + 1;
  const baseIndent = /^[ \t]*/u.exec(text.slice(lineStart))?.[0] ?? '';
  const entryIndent = baseIndent + indentUnit;
  const printed =
    compact || neighboursCompact
      ? compactPrint(value)
      : JSON.stringify(value, null, indentUnit)
          .split('\n')
          .map((line, i) => (i === 0 ? line : entryIndent + line))
          .join('\n');
  if (children.length === 0) {
    const open = node.offset + 1;
    const inserted = compact ? printed : `\n${entryIndent}${printed}\n${baseIndent}`;
    return text.slice(0, open) + inserted + text.slice(open);
  }
  if (index >= children.length) {
    const last = children[children.length - 1];
    const after = last.offset + last.length;
    const inserted = compact ? `, ${printed}` : `,\n${entryIndent}${printed}`;
    return text.slice(0, after) + inserted + text.slice(after);
  }
  const target = children[Math.max(0, index)];
  const inserted = compact ? `${printed}, ` : `${printed},\n${entryIndent}`;
  return text.slice(0, target.offset) + inserted + text.slice(target.offset);
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
      const fresh: Record<string, unknown> = {};
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
    if (!(key in next)) setAt(out.overrides, [...at, key], null);
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
    if (!(key in next)) out.overrides[key] = null;
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
  const out: DerivedMerge = { overrides: {}, arrays: [], questions: [] };
  if (strategy === 'deep-merge') diffDeep(base, next, [], prior, out);
  else diffShallow(base, next, prior, out);
  return out;
}
