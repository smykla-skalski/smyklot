/** Keep malformed wall times invalid instead of normalizing them into another hour. */
export function scheduleMinute(value: string): number {
  if (value === '24:00') return 1440;
  if (!/^(?:[01]\d|2[0-3]):[0-5]\d$/u.test(value)) return Number.NaN;
  const [hour = 0, minute = 0] = value.split(':').map(Number);
  return hour * 60 + minute;
}

/** Render a validated schedule boundary, including end-of-day minute 1440. */
export function scheduleMinuteText(value: number): string {
  return `${String(Math.floor(value / 60)).padStart(2, '0')}:${String(value % 60).padStart(2, '0')}`;
}
