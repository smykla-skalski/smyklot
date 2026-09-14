import { describe, expect, it } from 'vitest';
import { createPanelApi } from '../src/lib/api';
import { parseScheduleDatePreview } from '../src/lib/schedule-preview';

const interval = {
  start_minute: 135,
  end_minute: 165,
  available: true,
  opens_at: '2026-10-25T02:15:00+02:00',
  closes_at: '2026-10-25T02:45:00+01:00',
};
const preview = { date: '2026-10-25', timezone: 'Europe/Warsaw', windows: [interval] };
describe('schedule preview parsing', () => {
  it('preserves repeated-hour offsets and unavailable windows', () => {
    expect(parseScheduleDatePreview(preview)).toEqual(preview);
    const missing = {
      ...preview,
      windows: [{ start_minute: 135, end_minute: 165, available: false }],
    };
    expect(parseScheduleDatePreview(missing)).toEqual(missing);
  });
  it.each([
    null,
    {},
    { ...preview, windows: [{}] },
    { ...preview, windows: [{ ...interval, available: false }] },
    { ...preview, windows: [{ ...interval, closes_at: interval.opens_at }] },
    { ...preview, windows: [{ ...interval, opens_at: '2026-10-25T02:15:00' }] },
  ])('rejects malformed preview %j', (value) => {
    expect(() => parseScheduleDatePreview(value)).toThrow(TypeError);
  });
});

describe('schedule preview client', () => {
  const input = {
    date: preview.date,
    profile: { name: 'Unsaved', timezone: preview.timezone, windows: [], exceptions: [] },
  };
  it.each([
    { ...preview, date: '2026-10-26' },
    { ...preview, timezone: 'UTC' },
    { ...preview, windows: [{}] },
  ])('refuses unrelated or malformed results %j', async (body) => {
    const api = createPanelApi('', () => Promise.resolve(new Response(JSON.stringify(body))));
    await expect(api.previewScheduleDate(input)).rejects.toThrow();
  });
  it('carries the exact unsaved draft and cancellation signal', async () => {
    const controller = new AbortController();
    let called = false;
    const api = createPanelApi('', (url, init) => {
      called = true;
      expect(url).toBe('/api/v1/schedule-preview');
      expect(init?.method).toBe('POST');
      expect(JSON.parse(String(init?.body))).toEqual(input);
      expect(init?.signal).toBe(controller.signal);
      return Promise.resolve(new Response(JSON.stringify(preview)));
    });
    await expect(api.previewScheduleDate(input, controller.signal)).resolves.toEqual(preview);
    expect(called).toBe(true);
  });
});
