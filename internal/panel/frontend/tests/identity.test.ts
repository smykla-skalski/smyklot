import { describe, expect, it } from 'vitest';

import { handleLabel, monogram, readHandle } from '../src/lib/identity';

describe('readHandle', () => {
  it('renders the login as a handle', () => {
    expect(readHandle('github:https://api.github.com', 'bartsmykla')).toEqual({
      handle: '@bartsmykla',
      host: null,
    });
  });

  // Public GitHub is the only installation most panels ever see, so naming it
  // on every row would spend a line on the case that carries no information.
  it('says nothing about the installation when it is public GitHub', () => {
    expect(readHandle('github:https://api.github.com/', 'ada').host).toBeNull();
  });

  it('names an enterprise installation by host', () => {
    expect(readHandle('github:https://ghe.example.com/api/v3', 'ada')).toEqual({
      handle: '@ada',
      host: 'ghe.example.com',
    });
  });

  it('reads a provider it was not built for', () => {
    expect(readHandle('gitlab:https://gitlab.example.com', 'ada')).toEqual({
      handle: '@ada',
      host: 'gitlab.example.com',
    });
  });

  // Showing an unrecognised provider verbatim is worse than a clean handle but
  // better than hiding which installation an account came from.
  it('shows a provider it cannot parse verbatim', () => {
    expect(readHandle('something-else', 'ada')).toEqual({ handle: '@ada', host: 'something-else' });
  });

  it('claims no installation when the provider is blank', () => {
    expect(readHandle('', 'ada')).toEqual({ handle: '@ada', host: null });
  });
});

describe('handleLabel', () => {
  it('is the handle alone on public GitHub', () => {
    expect(handleLabel({ handle: '@ada', host: null })).toBe('@ada');
  });

  it('keeps a space either side of the separator', () => {
    expect(handleLabel({ handle: '@ada', host: 'ghe.example.com' })).toBe('@ada · ghe.example.com');
  });
});

describe('monogram', () => {
  it('takes the ends of a full name', () => {
    expect(monogram('Bart Smykla', 'bartsmykla')).toBe('BS');
    expect(monogram('Ada Lovelace King', 'ada')).toBe('AK');
  });

  it('takes one letter from a single word', () => {
    expect(monogram('ada', 'ada')).toBe('A');
  });

  it('falls back to the login when there is no display name', () => {
    expect(monogram('', 'ada')).toBe('A');
    expect(monogram('   ', 'ada')).toBe('A');
  });

  it('has something to show even with nothing to read', () => {
    expect(monogram('', '')).toBe('?');
  });

  // Splitting a name by code unit would cut a surrogate pair in half and render
  // a replacement glyph where the fallback avatar is meant to be.
  it('keeps a character that is more than one code unit whole', () => {
    expect(monogram('🌍 Earth', 'earth')).toBe('🌍E');
  });
});
