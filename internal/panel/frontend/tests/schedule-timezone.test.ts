import { describe, expect, it } from 'vitest';
import { createPanelApi } from '../src/lib/api';
import { parseScheduleTimezonePreview, timezoneLocalLabel } from '../src/lib/schedule-timezone';

const preview = {
  timezone: 'Europe/Warsaw',
  at: '2026-03-29T01:00:00Z',
  local_time: '2026-03-29T03:00:00+02:00',
  abbreviation: 'CEST',
  offset_seconds: 7200,
};

describe('authoritative timezone client', () => {
  it('keeps the server civil time even when the browser uses another zone', () => {
    const label = timezoneLocalLabel(preview);
    expect(label).toContain('03:00');
    expect(label).toContain('UTC+02:00');
  });
  it('rejects malformed responses rather than enabling save', () => {
    for (const value of [
      null,
      {},
      { ...preview, offset_seconds: '7200' },
      { ...preview, local_time: 'not a time' },
    ]) {
      expect(() => parseScheduleTimezonePreview(value)).toThrow();
    }
  });
  it('forwards cancellation and encodes the requested zone and instant', async () => {
    const controller = new AbortController();
    const api = createPanelApi('/panel', async (url, init) => {
      const parsed = new URL(url, 'http://localhost');
      expect(parsed.searchParams.get('timezone')).toBe(preview.timezone);
      expect(parsed.searchParams.get('at')).toBe(preview.at);
      expect(init?.signal).toBe(controller.signal);
      return new Response(JSON.stringify(preview));
    });
    expect(
      await api.previewScheduleTimezone(preview.timezone, preview.at, controller.signal),
    ).toEqual(preview);
  });
  it('rejects an answer for another timezone or instant', async () => {
    const api = createPanelApi('', async () => new Response(JSON.stringify(preview)));
    await expect(api.previewScheduleTimezone('UTC', preview.at)).rejects.toThrow('does not match');
    await expect(
      api.previewScheduleTimezone(preview.timezone, '2026-03-29T02:00:00Z'),
    ).rejects.toThrow('does not match');
  });
});
