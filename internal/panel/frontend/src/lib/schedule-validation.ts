import type { ScheduleProfile } from './types';

export interface ScheduleProblem {
  field: 'name' | 'windows' | 'exceptions';
  index?: number;
  message: string;
}

type Hours = Pick<ScheduleProfile, 'name' | 'windows' | 'exceptions'>;
type Range = { start_minute?: number; end_minute?: number };

/** Field-addressed calendar rules shared by the form and development API. */
export function scheduleHoursProblems(profile: Hours): ScheduleProblem[] {
  const problems: ScheduleProblem[] = [];
  if (profile.name.trim() === '') problems.push({ field: 'name', message: 'Enter a profile name' });
  if (profile.windows.length === 0 && profile.exceptions.length === 0) {
    problems.push({ field: 'windows', message: 'Add weekly hours or a date exception' });
  }
  for (const [index, window] of profile.windows.entries()) {
    if (!Number.isInteger(window.weekday) || window.weekday < 0 || window.weekday > 6) {
      problems.push({ field: 'windows', index, message: 'Choose a day of the week' });
    }
    if (!validRange(window)) {
      problems.push({
        field: 'windows',
        index,
        message: 'Closing time must be after opening time on the same day',
      });
    }
    if (
      profile.windows.some(
        (other, at) => at !== index && other.weekday === window.weekday && overlaps(window, other),
      )
    ) {
      problems.push({
        field: 'windows',
        index,
        message: 'These hours overlap another window on this day',
      });
    }
  }
  return [...problems, ...scheduleExceptionProblems(profile.exceptions)];
}

export function scheduleExceptionProblems(
  exceptions: ScheduleProfile['exceptions'],
): ScheduleProblem[] {
  const problems: ScheduleProblem[] = [];
  for (const [index, exception] of exceptions.entries()) {
    if (!validScheduleDate(exception.date)) {
      problems.push({ field: 'exceptions', index, message: 'Choose a valid calendar date' });
    }
    if (!exception.closed && !validRange(exception)) {
      problems.push({
        field: 'exceptions',
        index,
        message: 'Closing time must be after opening time on the same day',
      });
    }
    if (
      exceptions.some(
        (other, at) =>
          at !== index &&
          other.date === exception.date &&
          (exception.closed || other.closed || overlaps(exception, other)),
      )
    ) {
      problems.push({
        field: 'exceptions',
        index,
        message: 'Use either a closed day or non-overlapping hours for this date',
      });
    }
  }
  return problems;
}

/** Match Go's date-only calendar validation without JavaScript date rollover. */
export function validScheduleDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/u.test(value)) return false;
  const [year = 0, month = 0, day = 0] = value.split('-').map(Number);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return month >= 1 && month <= 12 && day >= 1 && day <= (days[month - 1] ?? 0);
}

function validRange(range: Range): boolean {
  return (
    Number.isInteger(range.start_minute) &&
    Number.isInteger(range.end_minute) &&
    range.start_minute! >= 0 &&
    range.start_minute! < 1440 &&
    range.end_minute! > 0 &&
    range.end_minute! <= 1440 &&
    range.start_minute! < range.end_minute!
  );
}

function overlaps(left: Range, right: Range): boolean {
  return (
    validRange(left) &&
    validRange(right) &&
    left.start_minute! < right.end_minute! &&
    right.start_minute! < left.end_minute!
  );
}
