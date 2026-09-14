import { describe, expect, it } from 'vitest';
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
