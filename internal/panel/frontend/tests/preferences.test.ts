import { describe, expect, it, vi } from 'vitest';

import {
  DEFAULT_SIDEBAR_DISPLAY,
  DEFAULT_THEME_DISPLAY,
  DEFAULT_TIME_DISPLAY,
  readLastInstallation,
  readSidebarDisplay,
  readThemeDisplay,
  readTimeDisplay,
  writeLastInstallation,
  writeSidebarDisplay,
  writeThemeDisplay,
  writeTimeDisplay,
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

  it('writes the selected display mode', () => {
    const setItem = vi.fn();

    writeTimeDisplay('absolute', { setItem });

    expect(setItem).toHaveBeenCalledWith('smyklot.panel.history.time-display', 'absolute');
  });

  it('continues when browser storage cannot be read or written', () => {
    expect(
      readTimeDisplay({
        getItem: () => {
          throw new DOMException('Storage is unavailable', 'SecurityError');
        },
      }),
    ).toBe(DEFAULT_TIME_DISPLAY);

    expect(() =>
      writeTimeDisplay('absolute', {
        setItem: () => {
          throw new DOMException('Storage is full', 'QuotaExceededError');
        },
      }),
    ).not.toThrow();
  });
});

describe('last installation preference', () => {
  it('restores a non-empty account slug', () => {
    expect(readLastInstallation({ getItem: () => 'smykla-skalski' })).toBe('smykla-skalski');
  });

  it('ignores an empty stored value', () => {
    expect(readLastInstallation({ getItem: () => '   ' })).toBeNull();
  });

  it('writes the selected account slug', () => {
    const setItem = vi.fn();

    writeLastInstallation('smykla-skalski', { setItem });

    expect(setItem).toHaveBeenCalledWith('smyklot.panel.last-installation', 'smykla-skalski');
  });

  it('continues when browser storage is unavailable', () => {
    expect(
      readLastInstallation({
        getItem: () => {
          throw new DOMException('Storage is unavailable', 'SecurityError');
        },
      }),
    ).toBeNull();

    expect(() =>
      writeLastInstallation('bartsmykla', {
        setItem: () => {
          throw new DOMException('Storage is full', 'QuotaExceededError');
        },
      }),
    ).not.toThrow();
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

  it('writes the selected sidebar display', () => {
    const setItem = vi.fn();

    writeSidebarDisplay('collapsed', { setItem });

    expect(setItem).toHaveBeenCalledWith('smyklot.panel.sidebar.display', 'collapsed');
  });

  it('continues when browser storage is unavailable', () => {
    expect(
      readSidebarDisplay({
        getItem: () => {
          throw new DOMException('Storage is unavailable', 'SecurityError');
        },
      }),
    ).toBe(DEFAULT_SIDEBAR_DISPLAY);

    expect(() =>
      writeSidebarDisplay('collapsed', {
        setItem: () => {
          throw new DOMException('Storage is full', 'QuotaExceededError');
        },
      }),
    ).not.toThrow();
  });
});

describe('theme preference', () => {
  it('uses the supplied system preference when no stored preference exists', () => {
    expect(readThemeDisplay({ getItem: () => null }, 'dark')).toBe('dark');
  });

  it.each(['light', 'dark'] as const)('restores the %s preference', (value) => {
    expect(readThemeDisplay({ getItem: () => value })).toBe(value);
  });

  it('ignores unsupported stored values', () => {
    expect(readThemeDisplay({ getItem: () => 'system' }, DEFAULT_THEME_DISPLAY)).toBe(
      DEFAULT_THEME_DISPLAY,
    );
  });

  it('writes the selected theme', () => {
    const setItem = vi.fn();

    writeThemeDisplay('dark', { setItem });

    expect(setItem).toHaveBeenCalledWith('smyklot.panel.theme', 'dark');
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

    expect(() =>
      writeThemeDisplay('light', {
        setItem: () => {
          throw new DOMException('Storage is full', 'QuotaExceededError');
        },
      }),
    ).not.toThrow();
  });
});
