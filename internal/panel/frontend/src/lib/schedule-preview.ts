import type { ScheduleProfile } from './types';

export type SchedulePreviewInput = {
  date: string;
  profile: Pick<ScheduleProfile, 'name' | 'timezone' | 'windows' | 'exceptions'>;
};
export interface ScheduleDatePreview {
  date: string;
  timezone: string;
  windows: {
    start_minute: number;
    end_minute: number;
    available: boolean;
    opens_at?: string;
    closes_at?: string;
  }[];
}

/** Validate the Go preview envelope before the mock or UI relies on it. */
export function parseScheduleDatePreview(value: unknown): ScheduleDatePreview {
  if (value === null || typeof value !== 'object') throw new TypeError('Invalid schedule preview');
  const record = value as Record<string, unknown>;
  if (
    typeof record.date !== 'string' ||
    typeof record.timezone !== 'string' ||
    !Array.isArray(record.windows)
  )
    throw new TypeError('Invalid schedule preview');
  const windows = record.windows.map((value: unknown) => {
    if (value === null || typeof value !== 'object') throw new TypeError('Invalid preview window');
    const item = value as Record<string, unknown>;
    const { start_minute, end_minute, available, opens_at, closes_at } = item;
    if (
      typeof start_minute !== 'number' ||
      !Number.isInteger(start_minute) ||
      start_minute < 0 ||
      typeof end_minute !== 'number' ||
      !Number.isInteger(end_minute) ||
      end_minute > 1440 ||
      start_minute >= end_minute ||
      typeof available !== 'boolean'
    )
      throw new TypeError('Invalid preview window');
    if (!available) {
      if (opens_at !== undefined || closes_at !== undefined)
        throw new TypeError('Unavailable preview has boundaries');
      return { start_minute, end_minute, available };
    }
    if (
      typeof opens_at !== 'string' ||
      typeof closes_at !== 'string' ||
      !/(Z|[+-]\d{2}:\d{2})$/u.test(opens_at) ||
      !/(Z|[+-]\d{2}:\d{2})$/u.test(closes_at) ||
      !Number.isFinite(Date.parse(opens_at)) ||
      !(Date.parse(closes_at) > Date.parse(opens_at))
    )
      throw new TypeError('Invalid preview boundaries');
    return { start_minute, end_minute, available, opens_at, closes_at };
  });
  return { date: record.date, timezone: record.timezone, windows };
}
