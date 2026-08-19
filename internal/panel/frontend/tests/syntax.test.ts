import { describe, expect, it } from 'vitest';

import { tokenize, type Token } from '../src/lib/syntax';

/** What a reader would say the line is made of, in order. */
const shape = (tokens: readonly Token[]): string =>
  tokens.map((token) => `${token.kind}:${token.text}`).join('|');

describe('tokenize [Unit]', () => {
  it('separates a JSON key from its value', () => {
    expect(shape(tokenize('  "name": "smyklot",', 'json'))).toBe(
      'plain:  |key:"name"|punct::|plain: |string:"smyklot"|punct:,',
    );
  });

  it('reads JSON literals as literals, not as text', () => {
    expect(shape(tokenize('  "enabled": true,', 'json'))).toContain('const:true');
    expect(shape(tokenize('  "count": -12.5', 'json'))).toContain('const:-12.5');
  });

  it('keeps a YAML comment whole, indentation and all', () => {
    expect(shape(tokenize('   # what this does', 'yaml'))).toBe('comment:   # what this does');
  });

  it('reads a YAML list item and a YAML pair', () => {
    expect(shape(tokenize('  - name: build', 'yaml'))).toBe(
      'punct:  - |key:name|punct::|string: build',
    );
    expect(shape(tokenize('  - ubuntu-latest', 'yaml'))).toBe('punct:  - |string:ubuntu-latest');
  });

  it('reads a TOML table header as structure and a pair as a pair', () => {
    expect(shape(tokenize('[merge]', 'toml'))).toBe('heading:[merge]');
    expect(shape(tokenize('method = "squash"', 'toml'))).toBe(
      'key:method|punct: = |string:"squash"',
    );
  });

  it('reads a Markdown heading whole and a code span inside prose', () => {
    expect(shape(tokenize('## Releasing', 'markdown'))).toBe('heading:## Releasing');
    expect(shape(tokenize('- run `mise run ci` first', 'markdown'))).toBe(
      'punct:- |plain:run |const:`mise run ci`|plain: first',
    );
  });

  it('gives an empty line no tokens at all, in every language', () => {
    for (const language of ['json', 'yaml', 'toml', 'markdown'] as const) {
      expect(tokenize('', language)).toEqual([]);
    }
  });

  it('loses no characters, whatever the line is', () => {
    const lines = [
      '{"a": [1, 2], "b": null} // trailing',
      '  key: "quoted: value"',
      'name = 12 # why',
      '**bold** and `code` and plain',
      '\t\tindented with tabs',
    ];
    for (const line of lines) {
      for (const language of ['json', 'yaml', 'toml', 'markdown'] as const) {
        expect(
          tokenize(line, language)
            .map((token) => token.text)
            .join(''),
        ).toBe(line);
      }
    }
  });
});
