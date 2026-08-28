import {
  FORMATTING_FIELDS,
  FORMATTING_GROUPS,
  FORMATTING_PRESETS,
  type FormattingField,
  type FormattingPatch,
  type FormattingPolicy,
  type FormattingPreset,
  type FormattingSources,
} from './formatting.generated.ts';

export type {
  FormattingField,
  FormattingFieldKey,
  FormattingGroup,
  FormattingPatch,
  FormattingPolicy,
  FormattingPreset,
  FormattingSources,
} from './formatting.generated.ts';

export type FormattingLeafValue = string | number;

interface DefinitionNode {
  readonly children: ReadonlyMap<string, DefinitionNode>;
  readonly field?: FormattingField;
}

interface MutableDefinitionNode {
  children: Map<string, MutableDefinitionNode>;
  field?: FormattingField;
}

const DEFINITION = buildDefinition();

/** Parse a complete effective policy at an API or persisted-draft boundary. */
export function parseFormattingPolicy(value: unknown): FormattingPolicy | null {
  return parseNode(value, DEFINITION, true) as FormattingPolicy | null;
}

/** Parse one sparse formatting layer, preserving explicit `preserve` values. */
export function parseFormattingPatch(value: unknown): FormattingPatch | null {
  return parseNode(value, DEFINITION, false) as FormattingPatch | null;
}

export function cloneFormattingPolicy(policy: FormattingPolicy): FormattingPolicy {
  const parsed = parseFormattingPolicy(policy);
  if (parsed === null) throw new TypeError('formatting policy is invalid');
  return parsed;
}

export function cloneFormattingPatch(patch: FormattingPatch): FormattingPatch {
  const parsed = parseFormattingPatch(patch);
  if (parsed === null) throw new TypeError('formatting patch is invalid');
  return parsed;
}

export function defaultFormattingPolicy(): FormattingPolicy {
  return cloneFormattingPolicy(FORMATTING_PRESETS.preserve);
}

/** Apply a layer exactly as the backend does: preset reset first, siblings second. */
export function applyFormattingPatch(
  base: FormattingPolicy,
  patch: FormattingPatch,
): FormattingPolicy {
  const validPatch = cloneFormattingPatch(patch);
  const preset = validPatch.preset;
  const resolved =
    preset === undefined
      ? cloneFormattingPolicy(base)
      : cloneFormattingPolicy(FORMATTING_PRESETS[preset]);
  const target = resolved as unknown as Record<string, unknown>;
  for (const field of FORMATTING_FIELDS) {
    const value = formattingPatchValue(validPatch, field);
    if (value !== undefined) setAtPath(target, field.path, value);
  }
  return resolved;
}

/** Convert a complete policy into a layer that explicitly sets every leaf. */
export function completeFormattingPatch(policy: FormattingPolicy): FormattingPatch {
  return cloneFormattingPolicy(policy) as FormattingPatch;
}

/** Return the smallest layer that resolves the base policy to the requested policy. */
export function formattingPolicyPatch(
  base: FormattingPolicy,
  resolved: FormattingPolicy,
): FormattingPatch {
  let patch: FormattingPatch = {};
  let comparison = base;
  if (resolved.preset !== base.preset) {
    const preset = FORMATTING_FIELDS[0];
    patch = setFormattingPatchValue(patch, preset, resolved.preset);
    comparison = FORMATTING_PRESETS[resolved.preset];
  }
  for (const field of FORMATTING_FIELDS.slice(1)) {
    const value = formattingPolicyValue(resolved, field);
    if (value !== formattingPolicyValue(comparison, field)) {
      patch = setFormattingPatchValue(patch, field, value);
    }
  }
  return patch;
}

export function formattingPoliciesEqual(left: FormattingPolicy, right: FormattingPolicy): boolean {
  return FORMATTING_FIELDS.every(
    (field) => formattingPolicyValue(left, field) === formattingPolicyValue(right, field),
  );
}

export function formattingPatchesEqual(left: FormattingPatch, right: FormattingPatch): boolean {
  return FORMATTING_FIELDS.every(
    (field) => formattingPatchValue(left, field) === formattingPatchValue(right, field),
  );
}

export function formattingOverrideCount(patch: FormattingPatch): number {
  return FORMATTING_FIELDS.reduce(
    (count, field) => count + (formattingPatchValue(patch, field) === undefined ? 0 : 1),
    0,
  );
}

export function formattingPolicyValue(
  policy: FormattingPolicy,
  field: FormattingField,
): FormattingLeafValue {
  const value = valueAtPath(policy as unknown as Record<string, unknown>, field.path);
  if (typeof value !== 'string' && typeof value !== 'number') {
    throw new TypeError(`formatting policy is missing ${field.key}`);
  }
  return value;
}

export function formattingPatchValue(
  patch: FormattingPatch,
  field: FormattingField,
): FormattingLeafValue | undefined {
  const value = valueAtPath(patch as Record<string, unknown>, field.path);
  return typeof value === 'string' || typeof value === 'number' ? value : undefined;
}

export function formattingField(key: string): FormattingField | undefined {
  return FORMATTING_FIELDS.find((field) => field.key === key);
}

export function isFormattingPreset(value: unknown): value is FormattingPreset {
  return typeof value === 'string' && Object.hasOwn(FORMATTING_PRESETS, value);
}

export function setFormattingPolicyValue(
  policy: FormattingPolicy,
  field: FormattingField,
  value: FormattingLeafValue,
): FormattingPolicy {
  const complete = setFormattingPatchValue(completeFormattingPatch(policy), field, value);
  const parsed = parseFormattingPolicy(complete);
  if (parsed === null) throw new TypeError(`invalid value for ${field.key}`);
  return parsed;
}

/** Return a canonical patch with one leaf set, or omitted when value is undefined. */
export function setFormattingPatchValue(
  patch: FormattingPatch,
  field: FormattingField,
  value: FormattingLeafValue | undefined,
): FormattingPatch {
  const next = cloneFormattingPatch(patch) as Record<string, unknown>;
  if (value === undefined) {
    deleteAtPath(next, field.path);
  } else {
    setAtPath(next, field.path, value);
  }
  const parsed = parseFormattingPatch(next);
  if (parsed === null) throw new TypeError(`invalid value for ${field.key}`);
  return parsed;
}

export function formattingSources<Source extends string>(
  source: Source,
): FormattingSources<Source> {
  const policy = defaultFormattingPolicy() as unknown as Record<string, unknown>;
  replaceLeaves(policy, source);
  return policy as FormattingSources<Source>;
}

export function applyFormattingSources<Source extends string>(
  base: FormattingSources<Source>,
  patch: FormattingPatch,
  source: Source,
): FormattingSources<Source> {
  const validPatch = cloneFormattingPatch(patch);
  const next = cloneRecord(base as unknown as Record<string, unknown>);
  if (validPatch.preset !== undefined) replaceLeaves(next, source);
  for (const field of FORMATTING_FIELDS) {
    if (formattingPatchValue(validPatch, field) !== undefined) {
      setAtPath(next, field.path, source);
    }
  }
  return next as FormattingSources<Source>;
}

function buildDefinition(): DefinitionNode {
  const root: MutableDefinitionNode = { children: new Map() };
  for (const field of FORMATTING_FIELDS) {
    let node = root;
    for (const part of field.path) {
      let child = node.children.get(part);
      if (child === undefined) {
        child = { children: new Map() };
        node.children.set(part, child);
      }
      node = child;
    }
    if (node.children.size > 0 || node.field !== undefined) {
      throw new TypeError(`duplicate generated formatting path: ${field.path.join('.')}`);
    }
    node.field = field;
  }
  return root;
}

function parseNode(
  value: unknown,
  definition: DefinitionNode,
  complete: boolean,
): Record<string, unknown> | null {
  if (!isPlainRecord(value)) return null;
  if (Object.keys(value).some((key) => !definition.children.has(key))) return null;

  const parsed: Record<string, unknown> = {};
  for (const [key, child] of definition.children) {
    if (!Object.hasOwn(value, key)) {
      if (complete) return null;
      continue;
    }
    const candidate = value[key];
    if (child.field !== undefined) {
      if (!validLeaf(child.field, candidate)) return null;
      parsed[key] = candidate;
      continue;
    }
    const nested = parseNode(candidate, child, complete);
    if (nested === null) return null;
    parsed[key] = nested;
  }
  return parsed;
}

function validLeaf(field: FormattingField, value: unknown): value is FormattingLeafValue {
  if (field.kind === 'int') {
    return (
      typeof value === 'number' &&
      Number.isSafeInteger(value) &&
      value >= field.minimum &&
      value <= field.maximum
    );
  }
  return typeof value === 'string' && field.options.some((option) => option === value);
}

function valueAtPath(root: Record<string, unknown>, path: readonly string[]): unknown {
  let current: unknown = root;
  for (const part of path) {
    if (!isPlainRecord(current)) return undefined;
    current = current[part];
  }
  return current;
}

function setAtPath(
  root: Record<string, unknown>,
  path: readonly string[],
  value: FormattingLeafValue,
): void {
  let current = root;
  for (const part of path.slice(0, -1)) {
    const existing = current[part];
    if (isPlainRecord(existing)) {
      current = existing;
    } else {
      const nested: Record<string, unknown> = {};
      current[part] = nested;
      current = nested;
    }
  }
  const leaf = path.at(-1);
  if (leaf !== undefined) current[leaf] = value;
}

function deleteAtPath(root: Record<string, unknown>, path: readonly string[]): void {
  const parents: Array<[Record<string, unknown>, string]> = [];
  let current = root;
  for (const part of path.slice(0, -1)) {
    const nested = current[part];
    if (!isPlainRecord(nested)) return;
    parents.push([current, part]);
    current = nested;
  }
  const leaf = path.at(-1);
  if (leaf === undefined) return;
  delete current[leaf];
  for (const [parent, key] of parents.toReversed()) {
    const nested = parent[key];
    if (isPlainRecord(nested) && Object.keys(nested).length === 0) delete parent[key];
  }
}

function replaceLeaves(value: Record<string, unknown>, replacement: string): void {
  for (const [key, child] of Object.entries(value)) {
    if (isPlainRecord(child)) replaceLeaves(child, replacement);
    else value[key] = replacement;
  }
}

function cloneRecord(value: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(value).map(([key, child]) => [
      key,
      isPlainRecord(child) ? cloneRecord(child) : child,
    ]),
  );
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false;
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

export { FORMATTING_FIELDS, FORMATTING_GROUPS, FORMATTING_PRESETS };
