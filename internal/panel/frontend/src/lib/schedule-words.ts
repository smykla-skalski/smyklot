import type { ScheduleProfile } from './types';

type Window = ScheduleProfile['windows'][number];

/** Monday first, because that is how a week of opening hours is read. */
const WEEK = [1, 2, 3, 4, 5, 6, 0];
const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
const DAY_MINUTES = 24 * 60;

function clock(minute: number): string {
  const hour = Math.floor(minute / 60);
  const rest = minute % 60;

  return `${hour}:${String(rest).padStart(2, '0')}`;
}

/**
 * Whether a profile is open every minute of the week.
 *
 * Read off the windows rather than off the name or the id: a profile somebody
 * built by hand that happens to cover the week is around the clock, and the
 * built-in one would still be if it were renamed.
 */
export function aroundTheClock(profile: ScheduleProfile): boolean {
  return WEEK.every((day) =>
    profile.windows.some(
      (window) =>
        window.weekday === day && window.start_minute === 0 && window.end_minute >= DAY_MINUTES,
    ),
  );
}

/** `during Europe business hours`, or `around the clock` for a profile that never closes. */
export function hoursPhrase(profile: ScheduleProfile | undefined, fallback: string): string {
  if (profile === undefined) return `during ${fallback}`;

  return aroundTheClock(profile) ? 'around the clock' : `during ${profile.name}`;
}

function sameHours(left: Window, right: Window): boolean {
  return left.start_minute === right.start_minute && left.end_minute === right.end_minute;
}

/**
 * A week of windows as somebody would say it: `Mon to Fri, 8:00 to 18:00`.
 *
 * Consecutive days that open and close at the same time are one run, so five
 * identical rows read as the working week rather than as five facts. Exceptions
 * are counted rather than listed - the profile's own editor is where they are
 * read one by one.
 */
export function windowsSentence(profile: ScheduleProfile): string {
  if (profile.windows.length === 0) return 'never open';

  const said = aroundTheClock(profile) ? 'never closed' : runs(profile.windows).join('; ');
  const exceptions = profile.exceptions.length;
  if (exceptions === 0) return said;

  return `${said}, plus ${exceptions === 1 ? 'one exception' : `${exceptions} exceptions`}`;
}

function runs(windows: readonly Window[]): string[] {
  const said: string[] = [];
  let run: { first: number; last: number; window: Window } | null = null;

  const close = (): void => {
    if (run === null) return;
    const days =
      run.first === run.last
        ? DAY_NAMES[run.first]
        : `${DAY_NAMES[run.first]} to ${DAY_NAMES[run.last]}`;
    said.push(`${days}, ${clock(run.window.start_minute)} to ${clock(run.window.end_minute)}`);
    run = null;
  };

  for (const day of WEEK) {
    /* One run per day: a day with two windows is rare enough to say twice rather
       than to invent a grammar for. */
    const open = windows.filter((window) => window.weekday === day);
    const first = open[0];
    if (first === undefined || open.length > 1) {
      close();
      for (const window of open) {
        said.push(
          `${DAY_NAMES[day]}, ${clock(window.start_minute)} to ${clock(window.end_minute)}`,
        );
      }
      continue;
    }
    if (run !== null && sameHours(run.window, first)) {
      run.last = day;
      continue;
    }
    close();
    run = { first: day, last: day, window: first };
  }
  close();

  return said;
}
