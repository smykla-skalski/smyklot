import { describe, expect, it } from 'vitest';
import { parseRequestJSON } from '../dev/request-json.js';

const runtime = (text: string) => parseRequestJSON(text, ['bot_config']);

describe('mock request JSON decoding [Unit]', () => {
  it.each([
    '{"bot_config":{"version":1,"version":1,"overrides":{}}}',
    '{"bot_config":{"quiet_success":false,"quiet_success":true}}',
    '{"bot_config":{"version":1,"overrides":{"quiet_success":false,"quiet_success":true}}}',
    String.raw`{"bot_config":{"version":1,"overrides":{"quiet_success":false,"quiet_\u0073uccess":true}}}`,
    '{"bot_config":{"version":1,"overrides":{"formatting":{"json":{"indent_width":2,"indent_width":4}}}}}',
    '{"bot_config":{"version":1,"overrides":{"command_aliases":{"a":"b","a":"c"}}}}',
    '{"bot_config":{"quiet_success":false,"quiet_success":true},"bot_config":null}',
  ])('rejects duplicate runtime fields before parsing loses them: %s', (text) => {
    expect(() => runtime(text)).toThrow(SyntaxError);
  });

  it('permits names reused in separate objects and duplicate-looking string content', () => {
    const value = {
      bot_config: {
        version: 1,
        overrides: {
          formatting: { json: { indent_width: 2 }, yaml: { indent_width: 4 } },
          command_prefix: '"x":1,"x":2',
        },
      },
    };
    expect(runtime(JSON.stringify(value))).toEqual(value);
  });

  it('retains the standard decoder behavior outside strict fields', () => {
    expect(runtime('{"expected_revision":1,"expected_revision":2,"bot_config":null}')).toEqual({
      expected_revision: 2,
      bot_config: null,
    });
    expect(parseRequestJSON('{"bot_config":{"x":1,"x":2}}')).toEqual({ bot_config: { x: 2 } });
  });

  it.each(['', '{', '{} {}', '{"bot_config":null,}', '{/* comment */"bot_config":null}'])(
    'rejects invalid JSON: %s',
    (text) => expect(() => runtime(text)).toThrow(SyntaxError),
  );

  it('preserves exact number tokens without boxing ordinary numbers', () => {
    const value = runtime('{"expected_revision":2,"large":9007199254740993,"fraction":1.50}');
    expect(value).toMatchObject({ expected_revision: 2 });
    expect(JSON.stringify(value)).toBe(
      '{"expected_revision":2,"large":9007199254740993,"fraction":1.50}',
    );
  });
});
