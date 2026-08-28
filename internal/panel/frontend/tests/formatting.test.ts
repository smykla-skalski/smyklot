import { describe, expect, it } from 'vitest';

import {
  FORMATTING_FIELDS,
  FORMATTING_GROUPS,
  FORMATTING_PRESETS,
  applyFormattingPatch,
  applyFormattingSources,
  defaultFormattingPolicy,
  formattingPatchValue,
  formattingOverrideCount,
  formattingPatchesEqual,
  formattingPoliciesEqual,
  formattingPolicyPatch,
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
    expect(FORMATTING_GROUPS.map(({ label }) => label)).toEqual([
      'Common',
      'JSON',
      'JSONC',
      'YAML',
      'TOML',
      'Markdown',
    ]);
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

  it('rejects prototype-pollution keys before applying a patch', () => {
    const attempts = [
      JSON.parse('{"__proto__":{"polluted":true}}') as unknown,
      JSON.parse('{"constructor":{"prototype":{"polluted":true}}}') as unknown,
      JSON.parse('{"json":{"__proto__":{"polluted":true}}}') as unknown,
    ];

    for (const attempt of attempts) {
      expect(parseFormattingPatch(attempt)).toBeNull();
      expect(() => applyFormattingPatch(defaultFormattingPolicy(), attempt as never)).toThrow(
        'formatting patch is invalid',
      );
    }
    expect(({} as { polluted?: boolean }).polluted).toBeUndefined();
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

  it('compares sparse layers without collapsing omission into preserve', () => {
    const omitted = {};
    const explicit = { json: { arrays: 'preserve' as const } };

    expect(formattingPatchesEqual(omitted, {})).toBe(true);
    expect(formattingPatchesEqual(omitted, explicit)).toBe(false);
    expect(formattingOverrideCount(explicit)).toBe(1);
  });

  it('derives a minimal layer from two complete policies', () => {
    const base = defaultFormattingPolicy();
    const resolved = applyFormattingPatch(base, {
      preset: 'conventional',
      json: { arrays: 'preserve' },
    });
    const patch = formattingPolicyPatch(base, resolved);

    expect(patch).toEqual({ preset: 'conventional', json: { arrays: 'preserve' } });
    expect(applyFormattingPatch(base, patch)).toEqual(resolved);
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
