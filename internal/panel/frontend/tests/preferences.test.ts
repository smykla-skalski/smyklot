import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  DEFAULT_SIDEBAR_DISPLAY,
  DEFAULT_THEME_DISPLAY,
  DEFAULT_TIME_DISPLAY,
  browserStorage,
  readLastWorkspace,
  readSidebarDisplay,
  readThemeDisplay,
  readTimeDisplay,
  resolveThemeDisplay,
  themeColor,
} from '../src/lib/preferences';

describe('history display preference', () => {
  it('uses the default when no stored preference exists', () => {
    expect(readTimeDisplay({ getItem: () => null })).toBe(DEFAULT_TIME_DISPLAY);
  });

  it.each(['relative', 'absolute'] as const)('restores the %s preference', (value) => {
    expect(readTimeDisplay({ getItem: () => value })).toBe(value);
  });

  it('ignores unsupported stored values', () => {
    expect(readTimeDisplay({ getItem: () => 'calendar' })).toBe(DEFAULT_TIME_DISPLAY);
  });

  it('continues when browser storage cannot be read', () => {
    expect(
      readTimeDisplay({
        getItem: () => {
          throw new DOMException('Storage is unavailable', 'SecurityError');
        },
      }),
    ).toBe(DEFAULT_TIME_DISPLAY);
  });
});

describe('last workspace preference', () => {
  it('restores a non-empty account slug', () => {
    expect(readLastWorkspace({ getItem: () => 'smykla-skalski' })).toBe('smykla-skalski');
  });

  it('ignores an empty stored value', () => {
    expect(readLastWorkspace({ getItem: () => '   ' })).toBeNull();
  });

  it('continues when browser storage is unavailable', () => {
    expect(
      readLastWorkspace({
        getItem: () => {
          throw new DOMException('Storage is unavailable', 'SecurityError');
        },
      }),
    ).toBeNull();
  });
});

describe('sidebar display preference', () => {
  it('uses the expanded default when no stored preference exists', () => {
    expect(readSidebarDisplay({ getItem: () => null })).toBe(DEFAULT_SIDEBAR_DISPLAY);
  });

  it.each(['expanded', 'collapsed'] as const)('restores the %s preference', (value) => {
    expect(readSidebarDisplay({ getItem: () => value })).toBe(value);
  });

  it('ignores unsupported stored values', () => {
    expect(readSidebarDisplay({ getItem: () => 'hidden' })).toBe(DEFAULT_SIDEBAR_DISPLAY);
  });

  it('continues when browser storage is unavailable', () => {
    expect(
      readSidebarDisplay({
        getItem: () => {
          throw new DOMException('Storage is unavailable', 'SecurityError');
        },
      }),
    ).toBe(DEFAULT_SIDEBAR_DISPLAY);
  });
});

describe('theme preference', () => {
  it('uses System when no stored preference exists', () => {
    expect(readThemeDisplay({ getItem: () => null })).toBe('system');
  });

  it.each(['system', 'light', 'dark'] as const)('restores the %s preference', (value) => {
    expect(readThemeDisplay({ getItem: () => value })).toBe(value);
  });

  it('ignores unsupported stored values', () => {
    expect(readThemeDisplay({ getItem: () => 'sepia' }, DEFAULT_THEME_DISPLAY)).toBe(
      DEFAULT_THEME_DISPLAY,
    );
  });

  it('resolves System from the current operating-system preference', () => {
    expect(resolveThemeDisplay('system', 'dark')).toBe('dark');
    expect(resolveThemeDisplay('system', 'light')).toBe('light');
    expect(resolveThemeDisplay('dark', 'light')).toBe('dark');
  });

  it.each([
    ['light', false, '#f5f7f6'],
    ['dark', false, '#0e1116'],
    ['light', true, '#f3f1f7'],
    ['dark', true, '#0f0d14'],
  ] as const)('uses the %s browser chrome color in Root=%s', (theme, rootConsole, color) => {
    expect(themeColor(theme, rootConsole)).toBe(color);
  });

  it('continues when browser storage is unavailable', () => {
    expect(
      readThemeDisplay(
        {
          getItem: () => {
            throw new DOMException('Storage is unavailable', 'SecurityError');
          },
        },
        'dark',
      ),
    ).toBe('dark');
  });
});

/**
 * Storage that is present and unusable, which is not the same as storage that throws.
 *
 * `window.localStorage` is typed non-optional, so nothing here is visible to the type checker: a
 * host can leave the accessor answering undefined, and Node's own Web Storage does exactly that
 * unless the process was given `--localstorage-file`. Reading it raises nothing, so the `catch`
 * that covers the disabled-storage case never runs and an undefined reaches the reader instead.
 */
describe('storage the browser declines to provide', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  /* Asserted on `browserStorage` rather than through a reader, because a reader cannot tell the
     two apart: it catches the TypeError from calling a missing `getItem` and returns the same
     default it would have returned from the guard. The rule under test is which path it took. */
  it.each([
    ['undefined', undefined],
    ['null', null],
    ['an object with no getItem', {}],
  ])('reads as no storage at all when localStorage is %s', (_case, localStorage) => {
    vi.stubGlobal('window', { localStorage });

    expect(browserStorage()).toBeNull();
  });

  it('reads as itself when the storage works', () => {
    const localStorage = { getItem: () => 'absolute' };
    vi.stubGlobal('window', { localStorage });

    expect(browserStorage()).toBe(localStorage);
    expect(readTimeDisplay()).toBe('absolute');
  });

  it('leaves every reader on its default when there is no storage', () => {
    vi.stubGlobal('window', { localStorage: undefined });

    expect(readTimeDisplay()).toBe(DEFAULT_TIME_DISPLAY);
    expect(readSidebarDisplay()).toBe(DEFAULT_SIDEBAR_DISPLAY);
    expect(readLastWorkspace()).toBeNull();
  });
});
