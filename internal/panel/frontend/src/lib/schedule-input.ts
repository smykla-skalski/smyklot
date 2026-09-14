import type { ScheduleProfile } from './types';
import { scheduleExceptionProblems } from './schedule-validation';

/** Keep malformed wall times invalid instead of normalizing them into another hour. */
export function scheduleMinute(value: string): number {
  if (value === '24:00') return 1440;
  if (!/^(?:[01]\d|2[0-3]):[0-5]\d$/u.test(value)) return Number.NaN;
  const [hour = 0, minute = 0] = value.split(':').map(Number);
  return hour * 60 + minute;
}

/** Transitional text adapter. Structured controls will use the same calendar validator. */
export function parseScheduleExceptions(value: string): ScheduleProfile['exceptions'] {
  const exceptions: ScheduleProfile['exceptions'] = [];
  const lines: number[] = [];
  for (const [index, raw] of value.split('\n').entries()) {
    const line = raw.trim();
    if (line === '') continue;
    const match = /^(\d{4}-\d{2}-\d{2})\s+(closed|\d{2}:\d{2}-\d{2}:\d{2})$/u.exec(line);
    if (match === null)
      throw new Error(
        `Date exceptions, line ${index + 1}: enter a date followed by closed or opening and closing times`,
      );
    const date = match[1]!;
    const span = match[2]!;
    lines.push(index + 1);
    if (span === 'closed') exceptions.push({ date, closed: true });
    else {
      const [start = '', end = ''] = span.split('-');
      exceptions.push({
        date,
        closed: false,
        start_minute: scheduleMinute(start),
        end_minute: scheduleMinute(end),
      });
    }
  }
  const problem = scheduleExceptionProblems(exceptions)[0];
  if (problem !== undefined)
    throw new Error(`Date exceptions, line ${lines[problem.index ?? 0]}: ${problem.message}`);
  return exceptions;
}
