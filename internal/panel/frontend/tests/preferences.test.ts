import { describe, expect, it, vi } from 'vitest';

import { DEFAULT_TIME_DISPLAY, readTimeDisplay, writeTimeDisplay } from '../src/lib/preferences';

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
