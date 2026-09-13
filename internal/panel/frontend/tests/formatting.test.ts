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
import { parseJson } from '../src/lib/merge';

describe('generated formatting contract [Unit]', () => {
  it('parses both generated presets as complete policies', () => {
    expect(parseFormattingPolicy(FORMATTING_PRESETS.preserve)).toEqual(FORMATTING_PRESETS.preserve);
    expect(parseFormattingPolicy(FORMATTING_PRESETS.conventional)).toEqual(
      FORMATTING_PRESETS.conventional,
    );
    expect(FORMATTING_FIELDS).toHaveLength(25);
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

  it('preserves zero as an explicit inline cap override and restores its source', () => {
    const inherited = applyFormattingPatch(defaultFormattingPolicy(), {
      common: { inline_max_chars: 48 },
    });
    const automatic = { common: { inline_max_chars: 0 } };
    const field = FORMATTING_FIELDS.find(
      ({ key }) => key === 'formatting.common.inline_max_chars',
    )!;

    expect(defaultFormattingPolicy().common.inline_max_chars).toBe(0);
    expect(applyFormattingPatch(inherited, {}).common.inline_max_chars).toBe(48);
    expect(applyFormattingPatch(inherited, automatic).common.inline_max_chars).toBe(0);
    expect(parseFormattingPatch(JSON.parse(JSON.stringify(automatic)))).toEqual(automatic);
    expect(formattingPatchesEqual({}, automatic)).toBe(false);
    expect(formattingPolicyPatch(inherited, applyFormattingPatch(inherited, automatic))).toEqual(
      automatic,
    );
    expect(
      applyFormattingSources(formattingSources('process'), automatic, 'target').common,
    ).toMatchObject({ inline_max_chars: 'target', line_width: 'process' });
    expect(setFormattingPatchValue(automatic, field, undefined)).toEqual({});
  });

  it.each([-1, 321, 1.5, Number.NaN, Number.POSITIVE_INFINITY, '32', null])(
    'rejects an invalid inline length cap: %s',
    (inline_max_chars) => {
      expect(parseFormattingPatch({ common: { inline_max_chars } })).toBeNull();
    },
  );

  it.each([0, 1, 320])('accepts an inline length cap at its valid boundaries: %s', (value) => {
    expect(parseFormattingPatch({ common: { inline_max_chars: value } })).toEqual({
      common: { inline_max_chars: value },
    });
  });

  it('reads bounded formatting metadata from lossless shared-file drafts', () => {
    const raw = parseJson('{"common":{"inline_max_chars":16,"line_width":100}}');
    expect(parseFormattingPatch(raw)).toEqual({
      common: { inline_max_chars: 16, line_width: 100 },
    });
    const field = FORMATTING_FIELDS.find(
      ({ key }) => key === 'formatting.common.inline_max_chars',
    )!;
    expect(formattingPatchValue({ common: { inline_max_chars: JSON.rawJSON('16') } }, field)).toBe(
      16,
    );
    expect(formattingPatchValue({ common: { inline_max_chars: JSON.rawJSON('0') } }, field)).toBe(
      0,
    );
  });

  it.each(['321', '-1', '1.5', '1e1', '9007199254740993', '"16"'])(
    'rejects an invalid raw inline cap literal: %s',
    (literal) => {
      expect(
        parseFormattingPatch(parseJson(`{"common":{"inline_max_chars":${literal}}}`)),
      ).toBeNull();
    },
  );

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
