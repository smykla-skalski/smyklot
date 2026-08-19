import { describe, expect, it } from 'vitest';

import {
  composeFile,
  derivePatch,
  deriveOverrides,
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

    //  1 {                          8       "matchManagers": [
    //  2   "automerge": false,       9         "gomod"
    //  3   "extends": [             10       ]
    //  4     "config:recommended"   11     }
    //  5   ],                       12   ],
    //  6   "packageRules": [        13   "schedule": [ … 16   ],
    //  7     {                      17   "timezone": "X"
    //
    // Keys sorted, because that is how the service writes a document back out.
    // The whole of the list, closing bracket included, because that is what
    // clearing the adjustment would take back.
    expect(markedLines(composed, { schedule: ['a', 'b'], timezone: 'X' })).toEqual([
      13, 14, 15, 16, 17,
    ]);
  });

  /**
   * A key is READ rather than matched as text. It used to be compared raw, so a
   * key carrying an escape never matched its own name and the line it names
   * went unmarked with nothing to say it had.
   */
  it('marks a key whose name is written with an escape', () => {
    const composed = '{\n  "a\\"b": 1,\n  "plain": 2\n}\n';

    expect(markedLines(composed, { 'a"b': 1 })).toEqual([2]);
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
    expect(parseJson('{oops')).toBeUndefined();
    expect(formatJson({ a: 1 })).toBe('{\n  "a": 1\n}\n');
  });

  /**
   * The service decodes with `UseNumber` and writes the digits back, so a
   * template holding `1.50` keeps it and an identifier past 2^53 keeps its
   * last four. Reading those as JavaScript numbers drew a file one digit from
   * the one the repository would get.
   */
  it('keeps a number as the digits it was written with', () => {
    const document = parseJson('{"rate": 1.50, "appId": 12345678901234567890}');

    expect(formatJson(document as JsonValue)).toBe(
      '{\n  "appId": 12345678901234567890,\n  "rate": 1.50\n}\n',
    );
  });

  /**
   * `result[key] = value` runs `__proto__`'s setter rather than storing a key
   * of that name, so the key vanished from the composed file and its value
   * became the object's prototype.
   */
  it('treats __proto__ as a key, in the template and in the adjustment', () => {
    const composed = mergePatch(parseJson('{"__proto__": {"kept": true}}') as JsonValue, {
      added: 1,
    });

    expect(formatJson(composed)).toBe(
      '{\n  "__proto__": {\n    "kept": true\n  },\n  "added": 1\n}\n',
    );
  });

  /**
   * Deriving walks a rule's path through the adjustment it is building, and a
   * level that is not there yet is built on the way.
   *
   * `candidate.__proto__` is not a missing level - it answers the prototype
   * every object in the page shares - so the walk arrived there and wrote the
   * repository's list onto it: `({}).list` became an array for every object,
   * and this reported success having stored nothing.
   */
  it('walks __proto__ as a key when it derives, not as a route to the prototype', () => {
    // Written back unchanged on purpose: that is what leaves the derived patch
    // empty, so the walk to the rule's list has no `__proto__` of its own to
    // step onto and takes the one every object shares instead. A case where the
    // repository does contribute something never reaches the defect, because
    // deriving has already put the key there.
    const held = parseJson('{"__proto__": {"list": ["theirs"]}}') as JsonValue;
    const spec = { arrays: [{ path: '$.__proto__.list', strategy: 'append' }] };
    const wanted = parseJson('{"__proto__": {"list": ["theirs"]}}') as JsonValue;

    const derived = deriveOverrides('renovate.json', held, spec, wanted);

    expect(Object.prototype).not.toHaveProperty('list');
    expect(derived.ok ? formatJson(derived.overrides) : derived.reason).toBe(
      '{\n  "__proto__": {\n    "list": []\n  }\n}\n',
    );
  });

  /**
   * The service refuses a shallow merge with a rule below the top level, so the
   * panel refuses it too rather than drawing a file no repository will hold.
   * Both halves matter: a shallow merge replaces a top-level key with the
   * adjustment's value whole, which is also the one shape where writing the
   * joined list back reached into the adjustment and grew it.
   */
  it('refuses a list rule below the top level of a shallow merge', () => {
    const held = parseJson('{"a": {"list": ["theirs"]}}') as JsonValue;
    const overrides = parseJson('{"a": {"list": ["ours"]}}') as JsonValue;
    const spec = {
      strategy: 'shallow-merge',
      overrides,
      arrays: [{ path: '$.a.list', strategy: 'append' }],
    };

    const composed = composeFile('renovate.json', held, spec);

    expect(composed.ok).toBe(false);
    expect(composed.ok ? '' : composed.reason).toContain('below the top level');
    expect(formatJson(overrides)).toBe('{\n  "a": {\n    "list": [\n      "ours"\n    ]\n  }\n}\n');
  });
});
