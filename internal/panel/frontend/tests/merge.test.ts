import { describe, expect, it } from 'vitest';

import {
  derivePatch,
  formatJson,
  markedLines,
  mergePatch,
  parseJson,
  patchedKeys,
  sharedArrays,
  type JsonValue,
} from '../src/lib/merge.js';

/**
 * The panel edits the RESULT and stores the difference. Everything here is
 * about that round trip holding: what somebody types is what the service will
 * compose, or the panel says it cannot store it.
 */
describe('merge [Unit]', () => {
  const template: JsonValue = {
    extends: ['config:recommended'],
    schedule: ['* 4 * * 6'],
    timezone: 'UTC',
    packageRules: [{ matchManagers: ['gomod'] }],
    automerge: false,
  };

  it('composes the file one repository ends up with', () => {
    expect(mergePatch(template, { timezone: 'Europe/Warsaw' })).toEqual({
      ...template,
      timezone: 'Europe/Warsaw',
    });
  });

  it('merges key by key, all the way down', () => {
    expect(mergePatch({ a: { b: 1, c: 2 } }, { a: { c: 3 } })).toEqual({ a: { b: 1, c: 3 } });
  });

  /** A null in a patch removes the key. That is the whole of deletion here. */
  it('removes a key the adjustment nulls', () => {
    expect(mergePatch(template, { automerge: null })).not.toHaveProperty('automerge');
  });

  /** RFC 7396 replaces a list rather than joining it, which is why the panel asks. */
  it('replaces a list rather than joining it', () => {
    expect(mergePatch(template, { schedule: ['* 4 * * 1-5'] })).toMatchObject({
      schedule: ['* 4 * * 1-5'],
    });
  });

  it('derives the smallest adjustment that turns one into the other', () => {
    const edited = { ...template, timezone: 'Europe/Warsaw', schedule: ['* 4 * * 1-5'] };

    expect(derivePatch(template, edited)).toEqual({
      timezone: 'Europe/Warsaw',
      schedule: ['* 4 * * 1-5'],
    });
  });

  it('says nothing to store where nothing changed', () => {
    expect(derivePatch(template, structuredClone(template))).toBeUndefined();
  });

  it('nulls a key the edit removed', () => {
    const rest = { ...(template as Record<string, JsonValue>) };
    delete rest.automerge;

    expect(derivePatch(template, rest)).toEqual({ automerge: null });
  });

  /**
   * The one thing RFC 7396 cannot say. Storing `{"x": null}` would mean "remove
   * x", so a patch that means something other than what somebody typed is
   * refused rather than written.
   */
  it('refuses to store a key set to null', () => {
    expect(derivePatch(template, { ...template, extra: null })).toBe('unsayable');
  });

  /** Round trip: what is derived, composed back, is what was typed. */
  it('round trips through the service’s own rule', () => {
    const edited = { ...template, timezone: 'Europe/Warsaw', automerge: true };
    const patch = derivePatch(template, edited);

    expect(mergePatch(template, patch as JsonValue)).toEqual(edited);
  });

  /**
   * From the adjustment, never by comparing two files line by line: the same
   * value written out twice can take a different number of lines, and a text
   * comparison marks all of them as the repository's.
   */
  it('marks the lines an adjustment decides, value and all', () => {
    const composed = formatJson(mergePatch(template, { schedule: ['a', 'b'], timezone: 'X' }));

    // 1 {                        6     "a",
    // 2   "extends": [           7     "b"
    // 3     "config:recommended" 8   ],
    // 4   ],                     9   "timezone": "X",
    // 5   "schedule": [         10   "packageRules": [ …
    //
    // The whole of the list, closing bracket included, because that is what
    // clearing the adjustment would take back.
    expect(markedLines(composed, { schedule: ['a', 'b'], timezone: 'X' })).toEqual([5, 6, 7, 8, 9]);
  });

  /** A brace inside a string is text, not a level that never closes. */
  it('counts structure and not the braces inside strings', () => {
    const composed = '{\n  "title": "{{DEFAULT_BRANCH}}",\n  "b": 1\n}\n';

    expect(markedLines(composed, { b: 1 })).toEqual([3]);
  });

  it('names the leaves an adjustment touches', () => {
    expect(
      patchedKeys({ timezone: 'X', hostRules: { matchHost: ['a'] }, automerge: null }),
    ).toEqual(['timezone', 'hostRules.matchHost', 'automerge']);
  });

  /** The one question a merge cannot answer, found where it arises. */
  it('finds the lists both sides set', () => {
    expect(sharedArrays(template, { packageRules: [{}], timezone: 'X' })).toEqual([
      '$.packageRules',
    ]);
    expect(sharedArrays(template, { nothingShared: [1] })).toEqual([]);
  });

  it('reads and writes the document the way a person types it', () => {
    expect(parseJson('{"a": 1}')).toEqual({ a: 1 });
    expect(parseJson('{oops')).toBeUndefined();
    expect(formatJson({ a: 1 })).toBe('{\n  "a": 1\n}\n');
  });
});
