import type { ScheduleProfile } from './types';
import { scheduleMinute, scheduleMinuteText } from './schedule-input';

export interface EditableException {
  id: string;
  date: string;
  closed: boolean;
  start: string;
  end: string;
}

export function editableExceptions(entries: ScheduleProfile['exceptions']): EditableException[] {
  return entries.map((entry, index) => ({
    id: `stored-exception-${index}`,
    date: entry.date,
    closed: entry.closed,
    start: scheduleMinuteText(entry.start_minute ?? 540),
    end: scheduleMinuteText(entry.end_minute ?? 1020),
  }));
}

export function exceptionInputs(
  entries: readonly EditableException[],
): ScheduleProfile['exceptions'] {
  return entries.map((entry) =>
    entry.closed
      ? { date: entry.date, closed: true }
      : {
          date: entry.date,
          closed: false,
          start_minute: scheduleMinute(entry.start),
          end_minute: scheduleMinute(entry.end),
        },
  );
}

export function exceptionDraft(entries: readonly EditableException[]): unknown {
  return entries.map(({ date, closed, start, end }) => ({ date, closed, start, end }));
}
