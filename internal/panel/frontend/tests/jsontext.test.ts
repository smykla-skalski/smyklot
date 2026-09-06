import { parse } from 'jsonc-parser';
import { describe, expect, it } from 'vitest';

import { mergedPreview } from '../src/lib/filemerge';
import { composeMergedText, deriveMerge, sameComposedContent } from '../src/lib/jsontext';

const TEMPLATE = [
  '{',
  '  "$schema": "https://docs.renovatebot.com/renovate-schema.json",',
  '  // Weekend runs keep review noise out of the working week',
  '  "extends": ["config:recommended"],',
  '  "schedule": ["* 4 * * 6"],',
  '  "timezone": "UTC",',
  '  "packageRules": [{ "matchDepTypes": ["devDependencies"], "automerge": true }],',
  '  "labels": ["dependencies"]',
  '}',
].join('\n');

describe('composeMergedText', () => {
  it.each(['append', 'prepend', 'replace'])(
    'deduplicates exact numeric values under %s without rounding neighboring integers',
    (strategy) => {
      const composed = composeMergedText('{"list":[{"amount":1.50},9007199254740992,-0]}', {
        overrides: {
          list: [
            { amount: JSON.rawJSON('15e-1') },
            JSON.rawJSON('9007199254740993'),
            JSON.rawJSON('9007199254740992'),
            JSON.rawJSON('0.0'),
            JSON.rawJSON('-0e5'),
          ],
        },
        arrays: [{ path: '$.list', strategy }],
        deduplicate: true,
      });
      const tokens = composed?.match(/-?\d+(?:\.\d+)?(?:e[+-]?\d+)?/gu);
      expect(tokens).toEqual(
        strategy === 'append'
          ? ['1.50', '9007199254740992', '-0', '9007199254740993']
          : ['15e-1', '9007199254740993', '9007199254740992', '0.0'],
      );
    },
  );

  it('writes a changed key without touching the rest of the file', () => {
    const composed = composeMergedText(TEMPLATE, {
      strategy: 'deep-merge',
      overrides: { timezone: 'Europe/Warsaw' },
    });
    expect(composed).toContain('"timezone": "Europe/Warsaw"');
    // The comment and the compact one-line array survive - the re-print never did this.
    expect(composed).toContain('// Weekend runs keep review noise');
    expect(composed).toContain('"extends": ["config:recommended"]');
  });

  it('appends under a list rule, keeping the template entries first', () => {
    const composed = composeMergedText(TEMPLATE, {
      strategy: 'deep-merge',
      overrides: { labels: ['renovate'] },
      arrays: [{ path: '$.labels', strategy: 'append' }],
    });
    expect(composed).not.toBeNull();
    const parsed = parse(composed ?? '') as { labels: string[] };
    expect(parsed.labels).toEqual(['dependencies', 'renovate']);
    expect(composed).toContain('"extends": ["config:recommended"]');
  });

  it('delegates appended object serialization to jsonc-parser', () => {
    const composed = composeMergedText(TEMPLATE, {
      strategy: 'deep-merge',
      overrides: { packageRules: [{ matchManagers: ['npm'] }] },
      arrays: [{ path: '$.packageRules', strategy: 'append' }],
    });
    expect(parse(composed ?? '')).toMatchObject({
      packageRules: [
        { matchDepTypes: ['devDependencies'], automerge: true },
        { matchManagers: ['npm'] },
      ],
    });
    expect(composed).toContain('// Weekend runs keep review noise');
  });

  it('prepends into a multiline list without reflowing what stands', () => {
    const template = ['{', '  "list": [', '    { "a": 1 },', '    { "b": 2 }', '  ]', '}'].join(
      '\n',
    );
    const composed = composeMergedText(template, {
      strategy: 'deep-merge',
      overrides: { list: [{ z: 9 }] },
      arrays: [{ path: '$.list', strategy: 'prepend' }],
    });
    expect(composed).toContain('    { "a": 1 },');
    expect(composed).toContain('    { "b": 2 }');
    expect(parse(composed ?? '')).toEqual({ list: [{ z: 9 }, { a: 1 }, { b: 2 }] });
  });

  it('agrees with the parsed-value merge on every strategy it speaks', () => {
    for (const strategy of ['deep-merge', 'shallow-merge']) {
      const merge = {
        strategy,
        overrides: {
          timezone: null,
          automerge: true,
          labels: ['renovate'],
          added: { nested: 1 },
        },
        arrays: [{ path: '$.labels', strategy: 'prepend' }],
      };
      const text = composeMergedText(TEMPLATE, merge);
      const printed = mergedPreview(TEMPLATE, merge);
      expect(text, strategy).not.toBeNull();
      expect(parse(text ?? ''), strategy).toEqual(JSON.parse(printed ?? ''));
    }
  });

  it('declines what it cannot compose honestly', () => {
    expect(composeMergedText(TEMPLATE, { strategy: 'markdown' })).toBeNull();
    expect(composeMergedText('not json', { strategy: 'deep-merge' })).toBeNull();
    expect(
      composeMergedText('{"duplicate":1,"duplicate":2}', {
        strategy: 'deep-merge',
      }),
    ).toBeNull();
  });

  it('treats prototype names as data without mutating object prototypes', () => {
    const overrides = JSON.parse('{"__proto__":{"polluted":true}}') as Record<string, unknown>;
    const composed = composeMergedText('{"safe":true}', {
      strategy: 'deep-merge',
      overrides,
    });

    const parsed = JSON.parse(composed ?? '') as Record<string, unknown>;
    expect(parsed.safe).toBe(true);
    expect(Object.hasOwn(parsed, '__proto__')).toBe(true);
    expect(parsed['__proto__']).toEqual({ polluted: true });
    expect(Object.hasOwn(Object.prototype, 'polluted')).toBe(false);
  });
});

describe('deriveMerge', () => {
  it.each(['deep-merge', 'shallow-merge'])(
    'refuses a new null field under %s rather than deriving a deletion',
    (strategy) => {
      expect(
        deriveMerge('{"id":0,"flag":false}', '{"id":null,"flag":false}', strategy, []),
      ).toBeNull();
    },
  );

  it.each([
    ['deep-merge', '{"id":null,"flag":false}', '{"id":null,"flag":true}'],
    ['deep-merge', '{"list":[1]}', '{"list":[null]}'],
    ['deep-merge', '{"nested":{"id":0}}', '{"nested":{"id":null}}'],
    ['shallow-merge', '{"nested":{"id":0}}', '{"nested":{"id":null}}'],
  ])(
    'round-trips representable null locations under %s: %s to %s',
    (strategy, template, edited) => {
      const derived = deriveMerge(template, edited, strategy, []);
      if (strategy === 'deep-merge' && edited.includes('nested')) {
        expect(derived).toBeNull();
        return;
      }
      expect(derived).not.toBeNull();
      expect(
        sameComposedContent(
          composeMergedText(template, { path: 'x.json', strategy, ...derived! })!,
          edited,
        ),
      ).toBe(true);
    },
  );

  it.each(['deep-merge', 'shallow-merge'])(
    'keeps a changed numeric literal that rounds to the template value under %s',
    (strategy) => {
      const result = deriveMerge(
        '{"id":9007199254740992,"amount":1.50}',
        '{"id":9007199254740993,"amount":1.5000}',
        strategy,
        [],
      );
      expect(JSON.stringify(result?.overrides)).toBe('{"id":9007199254740993,"amount":1.5000}');
    },
  );

  it.each(['append', 'prepend'])(
    'replaces a list when a %s contribution would retain the wrong numeric literal',
    (strategy) => {
      const wanted = strategy === 'append' ? '[9007199254740993,7]' : '[7,9007199254740993]';
      const result = deriveMerge('{"ids":[9007199254740992]}', `{"ids":${wanted}}`, 'deep-merge', [
        { path: '$.ids', strategy },
      ]);
      expect(JSON.stringify(result?.overrides)).toBe(`{"ids":${wanted}}`);
      expect(result?.arrays).toEqual([]);
      expect(result?.questions).toEqual([]);
    },
  );

  it('retains unchanged authored leaves during a partial nested edit', () => {
    const template = '{"automerge":false,"config":{"pinned":false,"name":"old"}}';
    const current = {
      text: template,
      merge: { overrides: { automerge: false, config: { pinned: false }, absent: null } },
    };
    expect(
      deriveMerge(template, template.replace('old', 'new'), 'deep-merge', [], { current }),
    ).toMatchObject({
      overrides: { automerge: false, config: { pinned: false, name: 'new' }, absent: null },
    });
  });

  it('retains scalar pins while changing an unrelated array strategy', () => {
    const template = '{"automerge":false,"labels":["base"]}';
    const current = {
      text: '{"automerge":false,"labels":["base","repo"]}',
      merge: {
        overrides: { automerge: false, labels: ['repo'] },
        arrays: [{ path: '$.labels', strategy: 'append' }],
      },
    };
    expect(deriveMerge(template, current.text, 'deep-merge', [], { current })).toMatchObject({
      overrides: { automerge: false, labels: ['base', 'repo'] },
      arrays: [],
    });
  });

  it('preserves authored rule order independently of property traversal', () => {
    const prior = [
      { path: '$.labels', strategy: 'append' },
      { path: '$.reviewers', strategy: 'append' },
    ];
    expect(
      deriveMerge(
        '{"reviewers":["base"],"labels":["base"]}',
        '{"reviewers":["base","reviewer"],"labels":["base","label"]}',
        'deep-merge',
        prior,
      )?.arrays,
    ).toEqual(prior);
  });

  it('does not restore a saved array contribution that requires different deduplication', () => {
    const template = '{"labels":["base"],"other":false}';
    const merge = {
      overrides: { labels: ['base'] },
      arrays: [{ path: '$.labels', strategy: 'append' }],
    };
    const current = {
      text: '{"labels":["base","base"],"other":false}',
      merge: { ...merge, deduplicate: false },
    };
    const saved = { text: template, merge: { ...merge, deduplicate: true } };
    expect(
      deriveMerge(template, template.replace('false', 'true'), 'deep-merge', merge.arrays, {
        current,
        saved,
      }),
    ).toMatchObject({ overrides: { other: true }, arrays: [] });
  });

  it('uses saved values only for leaves retained by the current intent', () => {
    const template = '{"kept":false,"removed":false,"other":"old"}';
    const saved = { text: template, merge: { overrides: { kept: false, removed: false } } };
    const current = {
      text: template.replace('"kept":false', '"kept":true'),
      merge: { overrides: { kept: true } },
    };
    expect(
      deriveMerge(template, template.replace('old', 'new'), 'deep-merge', [], { current, saved }),
    ).toMatchObject({ overrides: { kept: false, other: 'new' } });
  });

  it('does not restore nested pins through a deleted parent', () => {
    const template = '{"config":{"pinned":false},"other":false}';
    const current = {
      text: template,
      merge: { overrides: { config: { pinned: false, absent: null } } },
    };
    const result = deriveMerge(template, '{"other":true}', 'deep-merge', [], {
      current,
    });
    expect(result?.overrides).toEqual({ config: null, other: true });
  });

  it('retains exact authored numeric literals during an unrelated edit', () => {
    const template = '{"amount":1.50,"id":9007199254740993,"other":false}';
    const current = {
      text: template,
      merge: {
        overrides: { amount: JSON.rawJSON('1.50'), id: JSON.rawJSON('9007199254740993') },
      },
    };
    const result = deriveMerge(template, template.replace('false', 'true'), 'deep-merge', [], {
      current,
    });
    expect(JSON.stringify(result?.overrides)).toContain('"amount":1.50');
    expect(JSON.stringify(result?.overrides)).toContain('"id":9007199254740993');
  });

  it('retains shallow replacement objects only while their whole composed value is unchanged', () => {
    const template = '{"config":{"pinned":false},"other":false}';
    const current = { text: template, merge: { overrides: { config: { pinned: false } } } };
    expect(
      deriveMerge(
        template,
        template.replace('"other":false', '"other":true'),
        'shallow-merge',
        [],
        {
          current,
        },
      )?.overrides,
    ).toEqual({ config: { pinned: false }, other: true });
    expect(
      deriveMerge(template, '{"config":{"changed":true},"other":false}', 'shallow-merge', [], {
        current,
      })?.overrides,
    ).toEqual({ config: { changed: true } });
  });

  it.each(['deep-merge', 'shallow-merge'])(
    'retains authored list rules for paths absent from the template under %s',
    (strategy) => {
      const prior = [{ path: '$.ignorePaths', strategy: 'append' }];
      const derived = deriveMerge(
        '{"automerge":false}',
        '{"automerge":true,"ignorePaths":["generated/**"]}',
        strategy,
        prior,
      );
      expect(derived).toEqual({
        overrides: { automerge: true, ignorePaths: ['generated/**'] },
        arrays: prior,
        questions: [],
      });
    },
  );

  it('retains a nested list rule when its whole parent is absent from the template', () => {
    const prior = [{ path: '$.tools.ignorePaths', strategy: 'prepend' }];
    expect(
      deriveMerge('{}', '{"tools":{"ignorePaths":["generated/**"]}}', 'deep-merge', prior),
    ).toEqual({
      overrides: { tools: { ignorePaths: ['generated/**'] } },
      arrays: prior,
      questions: [],
    });
  });

  it('round-trips: compose then derive gives the override back', () => {
    const merge = {
      strategy: 'deep-merge',
      overrides: { timezone: 'Europe/Warsaw', schedule: null },
    };
    const composed = composeMergedText(TEMPLATE, merge) ?? '';
    const derived = deriveMerge(TEMPLATE, composed, 'deep-merge', []);
    expect(derived?.overrides).toEqual({ timezone: 'Europe/Warsaw', schedule: null });
    expect(derived?.arrays).toEqual([]);
  });

  it('an unedited copy derives no override at all', () => {
    const derived = deriveMerge(TEMPLATE, TEMPLATE, 'deep-merge', []);
    expect(derived?.overrides).toEqual({});
    expect(derived?.questions).toEqual([]);
  });

  it('asks about a list that grew at the end, and answers change the override', () => {
    const edited = composeMergedText(TEMPLATE, {
      strategy: 'deep-merge',
      overrides: { labels: ['dependencies', 'renovate'] },
    });
    const asked = deriveMerge(TEMPLATE, edited ?? '', 'deep-merge', []);
    // Without an answer the merge does what it always does: replace.
    expect(asked?.overrides).toEqual({ labels: ['dependencies', 'renovate'] });
    expect(asked?.questions).toEqual([
      { path: '$.labels', canAppend: true, canPrepend: false, chosen: 'replace' },
    ]);

    const answered = deriveMerge(TEMPLATE, edited ?? '', 'deep-merge', [
      { path: '$.labels', strategy: 'append' },
    ]);
    expect(answered?.overrides).toEqual({ labels: ['renovate'] });
    expect(answered?.arrays).toEqual([{ path: '$.labels', strategy: 'append' }]);
    expect(answered?.questions[0]?.chosen).toBe('append');
  });

  it('a reordered list is a replacement, with no question to ask', () => {
    const edited = composeMergedText(TEMPLATE, {
      strategy: 'deep-merge',
      overrides: { labels: ['renovate', 'dependencies-x'] },
    });
    const derived = deriveMerge(TEMPLATE, edited ?? '', 'deep-merge', [
      { path: '$.labels', strategy: 'append' },
    ]);
    expect(derived?.overrides).toEqual({ labels: ['renovate', 'dependencies-x'] });
    expect(derived?.arrays).toEqual([]);
    expect(derived?.questions).toEqual([]);
  });

  it('derives nested changes as a nested patch under deep-merge', () => {
    const edited = TEMPLATE.replace('"automerge": true', '"automerge": false');
    const derived = deriveMerge(TEMPLATE, edited, 'deep-merge', []);
    // packageRules is a list of records - a changed entry replaces the list.
    expect(derived?.overrides).toHaveProperty('packageRules');
  });

  it('round-trips service paths whose keys contain dots', () => {
    const template = '{ "host.rules": ["base"] }';
    const merge = {
      strategy: 'deep-merge',
      overrides: { 'host.rules': ['repo'] },
      arrays: [{ path: '$.host\\.rules', strategy: 'append' }],
    };
    const composed = composeMergedText(template, merge) ?? '';

    expect(parse(composed)).toEqual({ 'host.rules': ['base', 'repo'] });
    expect(deriveMerge(template, composed, 'deep-merge', merge.arrays)?.arrays).toEqual(
      merge.arrays,
    );
  });

  it('refuses what is not JSON rather than guessing', () => {
    expect(deriveMerge(TEMPLATE, '{ broken', 'deep-merge', [])).toBeNull();
    expect(deriveMerge(TEMPLATE, '[]', 'deep-merge', [])).toBeNull();
    expect(deriveMerge(TEMPLATE, '{}', 'markdown', [])).toBeNull();
  });

  it('derives prototype-named keys into own data properties', () => {
    const derived = deriveMerge(
      '{"safe":true}',
      '{"safe":true,"__proto__":{"polluted":true}}',
      'deep-merge',
      [],
    );

    expect(derived).not.toBeNull();
    expect(Object.hasOwn(derived?.overrides ?? {}, '__proto__')).toBe(true);
    expect(Object.hasOwn(Object.prototype, 'polluted')).toBe(false);
  });
});

describe('sameComposedContent', () => {
  it('ignores JSONC layout, object order and comments when matching saved content', () => {
    expect(
      sameComposedContent('{"a":[1,2],"b":false}', '{ // note\n "b":false, "a": [1, 2,], }'),
    ).toBe(true);
  });
  it.each([
    ['{"a":1.50}', '{"a":1.5}'],
    ['{"a":9007199254740992}', '{"a":9007199254740993}'],
    ['{"a":false}', '{"a":true}'],
    ['{"a":[1,2]}', '{"a":[2,1]}'],
    ['{"a":1,"a":1}', '{"a":1}'],
    ['{"a":', '{"a":'],
  ])('does not mistake %s for %s', (left, right) => {
    expect(sameComposedContent(left, right)).toBe(false);
  });
});
