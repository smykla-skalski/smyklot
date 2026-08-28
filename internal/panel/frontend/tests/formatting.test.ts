import { describe, expect, it } from 'vitest';

import {
  FORMATTING_FIELDS,
  FORMATTING_PRESETS,
  applyFormattingPatch,
  applyFormattingSources,
  defaultFormattingPolicy,
  formattingPatchValue,
  formattingPoliciesEqual,
  formattingSources,
  parseFormattingPatch,
  parseFormattingPolicy,
  setFormattingPatchValue,
} from '../src/lib/formatting';

describe('generated formatting contract [Unit]', () => {
  it('parses both generated presets as complete policies', () => {
    expect(parseFormattingPolicy(FORMATTING_PRESETS.preserve)).toEqual(FORMATTING_PRESETS.preserve);
    expect(parseFormattingPolicy(FORMATTING_PRESETS.conventional)).toEqual(
      FORMATTING_PRESETS.conventional,
    );
    expect(FORMATTING_FIELDS).toHaveLength(24);
    expect(new Set(FORMATTING_FIELDS.map(({ key }) => key)).size).toBe(FORMATTING_FIELDS.length);
  });

  it('resets every leaf at the preset before applying explicit siblings', () => {
    const result = applyFormattingPatch(defaultFormattingPolicy(), {
      preset: 'conventional',
      common: { line_ending: 'preserve' },
      json: { arrays: 'preserve' },
    });

    expect(result).toMatchObject({
      preset: 'conventional',
      common: { indent_style: 'spaces', line_ending: 'preserve', final_newline: 'insert' },
      json: { arrays: 'preserve', objects: 'auto', key_order: 'preserve' },
      yaml: { sequences: 'auto', mappings: 'block' },
      toml: { arrays: 'auto' },
      markdown: { tables: 'align' },
    });
  });

  it('distinguishes omission from an explicit preserve value', () => {
    const compact = applyFormattingPatch(defaultFormattingPolicy(), {
      json: { arrays: 'compact' },
    });

    expect(applyFormattingPatch(compact, {}).json.arrays).toBe('compact');
    expect(applyFormattingPatch(compact, { json: { arrays: 'preserve' } }).json.arrays).toBe(
      'preserve',
    );
  });

  it('strictly rejects unknown, partial, and out-of-bounds documents', () => {
    expect(parseFormattingPatch({ json: { unknown: 'preserve' } })).toBeNull();
    expect(parseFormattingPatch({ common: { indent_width: 0 } })).toBeNull();
    expect(parseFormattingPatch({ common: { line_width: 321 } })).toBeNull();
    expect(parseFormattingPatch({ json: { arrays: 'wide' } })).toBeNull();
    expect(parseFormattingPolicy({ preset: 'preserve' })).toBeNull();
  });

  it('edits one leaf without retaining empty parent objects', () => {
    const arrays = FORMATTING_FIELDS.find(({ key }) => key === 'formatting.json.arrays')!;
    const patch = setFormattingPatchValue({}, arrays, 'expanded');

    expect(patch).toEqual({ json: { arrays: 'expanded' } });
    expect(formattingPatchValue(patch, arrays)).toBe('expanded');
    expect(setFormattingPatchValue(patch, arrays, undefined)).toEqual({});
    expect(() => setFormattingPatchValue({}, arrays, 'invalid')).toThrow(
      'invalid value for formatting.json.arrays',
    );
  });

  it('tracks preset resets and explicit sibling provenance leaf by leaf', () => {
    const initial = formattingSources<'process' | 'target'>('process');
    const resolved = applyFormattingSources(
      initial,
      { preset: 'conventional', common: { line_ending: 'preserve' } },
      'target',
    );

    expect(resolved.preset).toBe('target');
    expect(resolved.json.arrays).toBe('target');
    expect(resolved.common.line_ending).toBe('target');
    expect(Object.values(resolved.markdown)).toEqual(['target', 'target', 'target']);
  });

  it('does not mutate policies or patches supplied by callers', () => {
    const base = defaultFormattingPolicy();
    const patch = { json: { arrays: 'compact' as const } };
    const beforeBase = structuredClone(base);
    const beforePatch = structuredClone(patch);

    const result = applyFormattingPatch(base, patch);

    expect(base).toEqual(beforeBase);
    expect(patch).toEqual(beforePatch);
    expect(formattingPoliciesEqual(base, result)).toBe(false);
  });
});
