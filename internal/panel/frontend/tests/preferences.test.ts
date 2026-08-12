import { describe, expect, it } from 'vitest';

import {
  DEFAULT_SIDEBAR_DISPLAY,
  DEFAULT_THEME_DISPLAY,
  DEFAULT_TIME_DISPLAY,
  readLastInstallation,
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

describe('last installation preference', () => {
  it('restores a non-empty account slug', () => {
    expect(readLastInstallation({ getItem: () => 'smykla-skalski' })).toBe('smykla-skalski');
  });

  it('ignores an empty stored value', () => {
    expect(readLastInstallation({ getItem: () => '   ' })).toBeNull();
  });

  it('continues when browser storage is unavailable', () => {
    expect(
      readLastInstallation({
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
